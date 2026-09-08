// Package ble wraps tinygo.org/x/bluetooth with the scan / connect / characteristic-selection rules the vendor app
// uses for its "default" profile: prefer the Nordic UART service, write to 6e400002, listen on 6e400003.
package ble

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	// NamePrefix is what every XGGF device's advertised local name starts with.
	NamePrefix = "XGGF"

	// DefaultScanTimeout bounds how long Scan and Find wait.
	DefaultScanTimeout = 15 * time.Second
)

// Generic GAP/GATT services the app skips when picking a service to write to.
var genericServices = map[uint16]bool{0x1800: true, 0x1801: true}

// Options selects the device and the characteristics to use.
type Options struct {
	// Name is a case-insensitive prefix match on the advertised local name. Ignored when Address is set.
	Name string
	// Address is an exact match on the platform address (MAC on Linux/Windows, CoreBluetooth UUID on macOS).
	Address string
	// ScanTimeout bounds discovery; zero means DefaultScanTimeout.
	ScanTimeout time.Duration

	// Service, WriteChar and NotifyChar override the automatic Nordic UART selection (any UUID text form).
	Service    string
	WriteChar  string
	NotifyChar string

	// Logf receives progress output; nil discards it.
	Logf func(format string, args ...any)
}

func (o Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

func (o Options) scanTimeout() time.Duration {
	if o.ScanTimeout <= 0 {
		return DefaultScanTimeout
	}
	return o.ScanTimeout
}

// Found is one scan result.
type Found struct {
	Address string
	Name    string
	RSSI    int16
	Result  bluetooth.ScanResult
}

var (
	adapterOnce sync.Once
	errAdapter  error
)

// Adapter enables the default adapter once and returns it. On macOS this waits for CoreBluetooth to power on and
// fails if the terminal has no Bluetooth permission or Bluetooth is off.
func Adapter() (*bluetooth.Adapter, error) {
	adapterOnce.Do(func() {
		errAdapter = bluetooth.DefaultAdapter.Enable()
	})
	if errAdapter != nil {
		return nil, fmt.Errorf("enabling bluetooth adapter: %w", errAdapter)
	}

	return bluetooth.DefaultAdapter, nil
}

// Scan runs discovery for timeout, calling found on the calling goroutine once per distinct address, and again for
// an address whose advertised name changes (the clock's name usually arrives in a later scan response than its
// first advertisement). When all is false only devices whose name starts with NamePrefix are reported. If found
// returns true the scan stops early.
func Scan(timeout time.Duration, all bool, found func(Found) bool) error {
	adapter, err := Adapter()
	if err != nil {
		return err
	}

	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}

	// the library invokes its callback on its own goroutine (the CoreBluetooth delegate on macOS, and on macOS it
	// can still fire after StopScan), so results are handed to this goroutine over a channel and the callback
	// gives up as soon as stop is closed rather than touching anything after Scan has returned
	var (
		results = make(chan Found, 16)
		stop    = make(chan struct{})
		errCh   = make(chan error, 1)
	)
	go func() {
		errCh <- adapter.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
			name := r.LocalName()
			if !all && !strings.HasPrefix(strings.ToUpper(name), NamePrefix) {
				return
			}

			select {
			case results <- Found{Address: r.Address.String(), Name: name, RSSI: r.RSSI, Result: r}:
			case <-stop:
			}
		})
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	seen := map[string]string{} // address -> name it was last reported with
loop:
	for {
		select {
		case f := <-results:
			if prev, dup := seen[f.Address]; dup && (f.Name == "" || f.Name == prev) {
				continue
			}
			seen[f.Address] = f.Name
			if found(f) {
				break loop
			}
		case <-timer.C:
			break loop
		case err := <-errCh:
			close(stop)
			if err != nil {
				return fmt.Errorf("scan: %w", err)
			}
			return nil
		}
	}
	close(stop)

	if err := adapter.StopScan(); err != nil {
		return fmt.Errorf("stopping scan: %w", err)
	}

	// wait for the scan goroutine to return so a following Scan does not race the library's
	// single-scan guard ("already calling Scan function"); Find restarts scans back to back
	<-errCh

	return nil
}

const (
	// findWindow is one short scan restart. CoreBluetooth delivers a device once per scan session and the local
	// name only arrives with the scan response, so a device that advertises its name intermittently is caught by
	// trying again in a fresh scan rather than sitting in one long scan.
	findWindow = 4 * time.Second
	// findMinWindow is the shortest scan worth starting; anything less just starts and stops the radio.
	findMinWindow = 500 * time.Millisecond
)

// Find scans until a device matching opts is seen, restarting the scan every findWindow until scanTimeout so an
// intermittently-named device is not missed.
func Find(opts Options) (Found, error) {
	var (
		match   Found
		matched bool
	)

	opts.logf("scanning for %s (up to %s)...", describeTarget(opts), opts.scanTimeout())

	deadline := time.Now().Add(opts.scanTimeout())
	for !matched {
		window := min(findWindow, time.Until(deadline))
		if window < findMinWindow {
			break
		}

		if err := Scan(window, true, func(f Found) bool {
			if !matches(opts, f) {
				return false
			}
			match, matched = f, true
			return true
		}); err != nil {
			return Found{}, err
		}
	}

	if !matched {
		return Found{}, fmt.Errorf("no device matching %s found within %s (run `scan --all` to see what is advertising; the clock often advertises with no name, so use --address)", describeTarget(opts), opts.scanTimeout())
	}

	opts.logf("found %s (%s, rssi %d)", match.Name, match.Address, match.RSSI)

	return match, nil
}

func matches(opts Options, f Found) bool {
	if opts.Address != "" {
		return strings.EqualFold(opts.Address, f.Address)
	}
	if opts.Name != "" {
		return strings.HasPrefix(strings.ToUpper(f.Name), strings.ToUpper(opts.Name))
	}
	return strings.HasPrefix(strings.ToUpper(f.Name), NamePrefix)
}

func describeTarget(opts Options) string {
	switch {
	case opts.Address != "":
		return "address " + opts.Address
	case opts.Name != "":
		return "name " + opts.Name + "*"
	default:
		return "name " + NamePrefix + "*"
	}
}

// Conn is an open connection. Open leaves only Services populated; Connect also resolves the write (and optional
// notify) characteristic.
type Conn struct {
	Device   bluetooth.Device
	Found    Found
	Services []bluetooth.DeviceService
	Service  bluetooth.UUID
	write    bluetooth.DeviceCharacteristic
	notify   *bluetooth.DeviceCharacteristic
}

// WriteUUID returns the characteristic commands are written to.
func (c *Conn) WriteUUID() bluetooth.UUID { return c.write.UUID() }

// NotifyUUID returns the characteristic notifications come from, or the zero UUID if none was found.
func (c *Conn) NotifyUUID() bluetooth.UUID {
	if c.notify == nil {
		return bluetooth.UUID{}
	}
	return c.notify.UUID()
}

// Open finds the device, connects and discovers its services, retrying discovery like the app does because some
// firmware answers an empty list right after connecting. Use Connect to also resolve the characteristics.
func Open(opts Options) (*Conn, error) {
	found, err := Find(opts)
	if err != nil {
		return nil, err
	}

	adapter, err := Adapter()
	if err != nil {
		return nil, err
	}

	opts.logf("connecting to %s...", found.Address)
	dev, err := adapter.Connect(found.Result.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", found.Address, err)
	}

	conn := &Conn{Device: dev, Found: found}
	for attempt := range 6 {
		time.Sleep(300 * time.Millisecond)
		if conn.Services, err = dev.DiscoverServices(nil); err != nil {
			_ = dev.Disconnect()
			return nil, fmt.Errorf("discovering services: %w", err)
		}
		if len(conn.Services) > 0 {
			break
		}
		opts.logf("service discovery returned nothing (attempt %d), retrying...", attempt+1)
	}
	if len(conn.Services) == 0 {
		_ = dev.Disconnect()
		return nil, errors.New("service discovery returned no services")
	}

	return conn, nil
}

// Connect opens the device and resolves the characteristics to use.
func Connect(opts Options) (*Conn, error) {
	conn, err := Open(opts)
	if err != nil {
		return nil, err
	}

	if err := conn.resolve(opts); err != nil {
		_ = conn.Close()
		return nil, err
	}

	opts.logf("using service %s, write %s, notify %s", conn.Service, conn.WriteUUID(), conn.NotifyUUID())

	return conn, nil
}

// Close disconnects.
func (c *Conn) Close() error {
	if err := c.Device.Disconnect(); err != nil {
		return fmt.Errorf("disconnecting: %w", err)
	}
	return nil
}

// Write sends one packet, with or without a GATT response. The app uses write-with-response.
func (c *Conn) Write(p []byte, withResponse bool) error {
	var err error
	if withResponse {
		_, err = c.write.Write(p)
	} else {
		_, err = c.write.WriteWithoutResponse(p)
	}
	if err != nil {
		return fmt.Errorf("writing %d bytes to %s: %w", len(p), c.write.UUID(), err)
	}

	return nil
}

// ErrNoNotify is returned by Listen when no notify characteristic was resolved.
var ErrNoNotify = errors.New("no notify characteristic available on this device")

// Listen enables notifications and calls cb for every value received until Close.
func (c *Conn) Listen(cb func([]byte)) error {
	if c.notify == nil {
		return ErrNoNotify
	}
	if err := c.notify.EnableNotifications(cb); err != nil {
		return fmt.Errorf("enabling notifications on %s: %w", c.notify.UUID(), err)
	}

	return nil
}

// resolve implements the app's selection rules, or the explicit overrides in opts. The library does not expose
// characteristic properties, so outside the Nordic UART characteristics there is no way to tell which one is
// writable; the first non-generic service carrying the UART RX characteristic wins, otherwise the user has to pick
// with --service/--write-char.
func (c *Conn) resolve(opts Options) error {
	if opts.Service != "" || opts.WriteChar != "" || opts.NotifyChar != "" {
		return c.resolveExplicit(opts)
	}

	for _, svc := range c.Services {
		if isGeneric(svc.UUID()) {
			continue
		}

		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			opts.logf("service %s: characteristic discovery failed: %v", svc.UUID(), err)
			continue
		}

		var write, notify *bluetooth.DeviceCharacteristic
		for i := range chars {
			switch chars[i].UUID() {
			case bluetooth.CharacteristicUUIDUARTRX:
				write = &chars[i]
			case bluetooth.CharacteristicUUIDUARTTX:
				notify = &chars[i]
			default:
			}
		}
		if write == nil {
			continue
		}

		c.Service, c.write, c.notify = svc.UUID(), *write, notify

		return nil
	}

	return fmt.Errorf("no service with a %s write characteristic found; run `dump` and pass --service/--write-char explicitly", bluetooth.CharacteristicUUIDUARTRX)
}

func (c *Conn) resolveExplicit(opts Options) error {
	if opts.Service == "" || opts.WriteChar == "" {
		return errors.New("--service and --write-char must be given together")
	}

	svcUUID, err := bluetooth.ParseUUID(opts.Service)
	if err != nil {
		return fmt.Errorf("parsing --service %q: %w", opts.Service, err)
	}
	writeUUID, err := bluetooth.ParseUUID(opts.WriteChar)
	if err != nil {
		return fmt.Errorf("parsing --write-char %q: %w", opts.WriteChar, err)
	}
	var notifyUUID bluetooth.UUID
	if opts.NotifyChar != "" {
		if notifyUUID, err = bluetooth.ParseUUID(opts.NotifyChar); err != nil {
			return fmt.Errorf("parsing --notify-char %q: %w", opts.NotifyChar, err)
		}
	}

	for _, svc := range c.Services {
		if svc.UUID() != svcUUID {
			continue
		}

		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			return fmt.Errorf("discovering characteristics of %s: %w", svc.UUID(), err)
		}

		var write, notify *bluetooth.DeviceCharacteristic
		for i := range chars {
			if chars[i].UUID() == writeUUID {
				write = &chars[i]
			}
			if opts.NotifyChar != "" && chars[i].UUID() == notifyUUID {
				notify = &chars[i]
			}
		}
		if write == nil {
			return fmt.Errorf("service %s has no characteristic %s", svcUUID, writeUUID)
		}
		if opts.NotifyChar != "" && notify == nil {
			return fmt.Errorf("service %s has no characteristic %s", svcUUID, notifyUUID)
		}

		c.Service, c.write, c.notify = svc.UUID(), *write, notify

		return nil
	}

	return fmt.Errorf("device has no service %s", svcUUID)
}

func isGeneric(u bluetooth.UUID) bool {
	return u.Is16Bit() && genericServices[u.Get16Bit()]
}

// Dump writes every service and characteristic (with a best-effort read of each value) to w.
func (c *Conn) Dump(w io.Writer) {
	// console output: a failed write to w is not actionable here, so the error is deliberately dropped once
	out := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }

	out("device %s (%s)\n", c.Found.Name, c.Found.Address)
	out("advertised services: %v\n", c.Found.Result.ServiceUUIDs())
	for _, m := range c.Found.Result.ManufacturerData() {
		out("manufacturer data: company 0x%04X data % X\n", m.CompanyID, m.Data)
	}
	out("%d services\n", len(c.Services))

	buf := make([]byte, 512)
	for _, svc := range c.Services {
		out("- service %s%s\n", svc.UUID(), annotateService(svc.UUID()))

		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			out("  ! characteristic discovery failed: %v\n", err)
			continue
		}
		for _, ch := range chars {
			out("  - characteristic %s%s\n", ch.UUID(), annotateChar(ch.UUID()))
			if mtu, err := ch.GetMTU(); err == nil {
				out("      mtu %d\n", mtu)
			}
			n, err := ch.Read(buf)
			switch {
			case err != nil:
				out("      read: %v\n", err)
			case n == 0:
				out("      read: (empty)\n")
			default:
				out("      read: % X  %q\n", buf[:n], Printable(buf[:n]))
			}
		}
	}
}

func annotateService(u bluetooth.UUID) string {
	switch {
	case u == bluetooth.ServiceUUIDNordicUART:
		return "  (Nordic UART service, expected)"
	case isGeneric(u):
		return "  (generic, skipped by the app)"
	default:
		return ""
	}
}

func annotateChar(u bluetooth.UUID) string {
	switch u {
	case bluetooth.CharacteristicUUIDUARTRX:
		return "  (UART RX: phone->device, commands are written here)"
	case bluetooth.CharacteristicUUIDUARTTX:
		return "  (UART TX: device->phone, notifications)"
	default:
		return ""
	}
}

// Printable renders b as ASCII with every non-printable byte replaced by a dot.
func Printable(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}
