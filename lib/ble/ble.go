// Package ble wraps tinygo.org/x/bluetooth with the scan / connect / characteristic-selection rules the vendor app
// uses for its "default" profile: prefer the Nordic UART service, write to 6e400002, listen on 6e400003.
package ble

import (
	"errors"
	"fmt"
	"io"
	"slices"
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

// Scan runs discovery for timeout, calling found once per distinct address. When all is false only devices whose
// name starts with NamePrefix are reported. If found returns true the scan stops early.
func Scan(timeout time.Duration, all bool, found func(Found) bool) error {
	adapter, err := Adapter()
	if err != nil {
		return err
	}

	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}

	var (
		mu   sync.Mutex
		seen = map[string]bool{}
		done = make(chan struct{})
		once sync.Once
	)
	finish := func() { once.Do(func() { close(done) }) }

	errCh := make(chan error, 1)
	go func() {
		errCh <- adapter.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
			name := r.LocalName()
			if !all && !strings.HasPrefix(strings.ToUpper(name), NamePrefix) {
				return
			}

			addr := r.Address.String()
			mu.Lock()
			dup := seen[addr]
			seen[addr] = true
			mu.Unlock()
			if dup {
				return
			}

			if found(Found{Address: addr, Name: name, RSSI: r.RSSI, Result: r}) {
				finish()
			}
		})
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		return nil
	}

	if err := adapter.StopScan(); err != nil {
		return fmt.Errorf("stopping scan: %w", err)
	}

	// wait for the scan goroutine to return so a following Scan does not race the library's
	// single-scan guard ("already calling Scan function"); Find restarts scans back to back
	<-errCh

	return nil
}

// findWindow is one short scan restart. CoreBluetooth delivers a device once per scan session and the local name
// only arrives with the scan response, so a device that advertises its name intermittently is caught by trying
// again in a fresh scan rather than sitting in one long scan.
const findWindow = 4 * time.Second

// Find scans until a device matching opts is seen, restarting the scan every findWindow until scanTimeout so an
// intermittently-named device is not missed.
func Find(opts Options) (Found, error) {
	var (
		match   Found
		matched bool
	)

	opts.logf("scanning for %s (up to %s)...", describeTarget(opts), opts.scanTimeout())

	deadline := time.Now().Add(opts.scanTimeout())
	for !matched && time.Now().Before(deadline) {
		window := findWindow
		if remaining := time.Until(deadline); remaining < window {
			window = remaining
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

// Conn is an open connection with the write (and optional notify) characteristic resolved.
type Conn struct {
	Device  bluetooth.Device
	Found   Found
	Service bluetooth.UUID
	write   bluetooth.DeviceCharacteristic
	notify  *bluetooth.DeviceCharacteristic
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

// Connect finds the device, connects, discovers services and resolves the characteristics.
func Connect(opts Options) (*Conn, error) {
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
	if err := conn.resolve(opts); err != nil {
		_ = dev.Disconnect()
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

// resolve implements the app's selection rules, or the explicit overrides in opts.
func (c *Conn) resolve(opts Options) error {
	// the app retries discovery a few times because some firmware answers an empty list right after connecting
	var (
		services []bluetooth.DeviceService
		err      error
	)
	for attempt := range 6 {
		time.Sleep(300 * time.Millisecond)
		services, err = c.Device.DiscoverServices(nil)
		if err != nil {
			return fmt.Errorf("discovering services: %w", err)
		}
		if len(services) > 0 {
			break
		}
		opts.logf("service discovery returned nothing (attempt %d), retrying...", attempt+1)
	}
	if len(services) == 0 {
		return errors.New("service discovery returned no services")
	}

	// overrides win outright
	if opts.Service != "" || opts.WriteChar != "" || opts.NotifyChar != "" {
		return c.resolveExplicit(opts, services)
	}

	// prefer the Nordic UART service, then anything non-generic, in discovery order
	slices.SortStableFunc(services, func(a, b bluetooth.DeviceService) int {
		return rank(a.UUID()) - rank(b.UUID())
	})

	for _, svc := range services {
		if isGeneric(svc.UUID()) {
			continue
		}

		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			opts.logf("service %s: characteristic discovery failed: %v", svc.UUID(), err)
			continue
		}
		if len(chars) == 0 {
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
		if write == nil && svc.UUID() != bluetooth.ServiceUUIDNordicUART {
			// the library does not expose characteristic properties, so outside NUS we cannot tell which one is
			// writable; make the user pick with --service/--write-char rather than guess
			continue
		}
		if write == nil {
			return fmt.Errorf("service %s has no %s write characteristic", svc.UUID(), bluetooth.CharacteristicUUIDUARTRX)
		}

		c.Service, c.write, c.notify = svc.UUID(), *write, notify

		return nil
	}

	return errors.New("no Nordic UART service found; run `dump` and pass --service/--write-char explicitly")
}

func (c *Conn) resolveExplicit(opts Options, services []bluetooth.DeviceService) error {
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

	for _, svc := range services {
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

func rank(u bluetooth.UUID) int {
	switch {
	case u == bluetooth.ServiceUUIDNordicUART:
		return 0
	case isGeneric(u):
		return 2
	default:
		return 1
	}
}

// Dump connects and writes every service and characteristic (with a best-effort read of each value) to w.
func Dump(opts Options, w io.Writer) error {
	found, err := Find(opts)
	if err != nil {
		return err
	}

	adapter, err := Adapter()
	if err != nil {
		return err
	}

	opts.logf("connecting to %s...", found.Address)
	dev, err := adapter.Connect(found.Result.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", found.Address, err)
	}
	defer func() { _ = dev.Disconnect() }()

	time.Sleep(300 * time.Millisecond)
	services, err := dev.DiscoverServices(nil)
	if err != nil {
		return fmt.Errorf("discovering services: %w", err)
	}

	fmt.Fprintf(w, "device %s (%s)\n", found.Name, found.Address)
	fmt.Fprintf(w, "advertised services: %v\n", found.Result.ServiceUUIDs())
	if md := found.Result.ManufacturerData(); len(md) > 0 {
		for _, m := range md {
			fmt.Fprintf(w, "manufacturer data: company 0x%04X data % X\n", m.CompanyID, m.Data)
		}
	}
	fmt.Fprintf(w, "%d services\n", len(services))

	buf := make([]byte, 512)
	for _, svc := range services {
		fmt.Fprintf(w, "- service %s%s\n", svc.UUID(), annotateService(svc.UUID()))

		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			fmt.Fprintf(w, "  ! characteristic discovery failed: %v\n", err)
			continue
		}
		for _, ch := range chars {
			fmt.Fprintf(w, "  - characteristic %s%s\n", ch.UUID(), annotateChar(ch.UUID()))
			if mtu, err := ch.GetMTU(); err == nil {
				fmt.Fprintf(w, "      mtu %d\n", mtu)
			}
			n, err := ch.Read(buf)
			switch {
			case err != nil:
				fmt.Fprintf(w, "      read: %v\n", err)
			case n == 0:
				fmt.Fprint(w, "      read: (empty)\n")
			default:
				fmt.Fprintf(w, "      read: % X  %q\n", buf[:n], printable(buf[:n]))
			}
		}
	}

	return nil
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

func printable(b []byte) string {
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
