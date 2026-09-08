package cli

import (
	"fmt"
	"time"

	"github.com/katbyte/acornvfd/lib/ble"
	"github.com/katbyte/acornvfd/lib/cout"
	"github.com/katbyte/acornvfd/lib/state"
	"github.com/katbyte/acornvfd/lib/xggf"
)

// sender writes labelled packets to a device, or only prints them in dry-run mode.
type sender struct {
	f    *FlagData
	conn *ble.Conn
	sent int
}

// withSender connects (unless --dry-run) and runs fn with a sender, then disconnects. Packets are built inside fn
// so time-based ones are computed after the connection is up, not before a scan.
func (f *FlagData) withSender(fn func(s *sender) error) error {
	s := &sender{f: f}

	if f.Send.DryRun {
		cout.Printf("<yellow>dry run</>: nothing will be sent\n")
	} else {
		conn, err := f.connect()
		if err != nil {
			return err
		}
		defer closeConn(conn)
		s.conn = conn
	}

	if err := fn(s); err != nil {
		return err
	}

	if s.sent > 0 && !f.Send.DryRun {
		cout.Printf("<green>ok</> sent %d packet%s\n", s.sent, plural(s.sent))
	}

	return nil
}

// Packet sends one framed packet, pausing --gap after any previous one.
func (s *sender) Packet(lp labelledPacket) error {
	return s.Bytes(lp.label, lp.packet.Bytes())
}

// Bytes sends arbitrary bytes, pausing --gap after any previous packet.
func (s *sender) Bytes(label string, b []byte) error {
	if s.sent > 0 && s.f.Send.Gap > 0 {
		time.Sleep(s.f.Send.Gap)
	}

	cout.Quietf("<white>%-28s</> <cyan>% X</>\n", label, b)

	if !s.f.Send.DryRun {
		if err := s.conn.Write(b, !s.f.Send.NoResponse); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	s.sent++

	return nil
}

// sendPackets is the one-shot helper for commands that know their packets up front.
func (f *FlagData) sendPackets(labelled ...labelledPacket) error {
	return f.withSender(func(s *sender) error {
		for _, lp := range labelled {
			if err := s.Packet(lp); err != nil {
				return err
			}
		}
		return nil
	})
}

func (f *FlagData) sendBytes(label string, b []byte) error {
	return f.withSender(func(s *sender) error { return s.Bytes(label, b) })
}

// open finds the device and connects, discovering services but not resolving characteristics (for dump).
func (f *FlagData) open() (*ble.Conn, error) {
	conn, err := ble.Open(f.BLEOptions())
	if err != nil {
		return nil, err
	}
	f.connected(conn)

	return conn, nil
}

// connect opens the device and resolves the write/notify characteristics.
func (f *FlagData) connect() (*ble.Conn, error) {
	conn, err := ble.Connect(f.BLEOptions())
	if err != nil {
		return nil, err
	}
	f.connected(conn)

	return conn, nil
}

// connected logs the connection and remembers the device's address so later runs reconnect without a name (the
// clock advertises its name only intermittently). Lamp commands never remember: a bare `acornvfd time` would
// otherwise reconnect to the lamp.
func (f *FlagData) connected(conn *ble.Conn) {
	cout.Verbosef("<gray>connected to %s (%s)</>\n", nameOr(conn.Found.Name), conn.Found.Address)
	if !f.noRemember {
		rememberDevice(f.StateFile, conn.Found)
	}
}

// closeConn disconnects, warning rather than failing the command when the disconnect itself errors.
func closeConn(conn *ble.Conn) {
	if err := conn.Close(); err != nil {
		cout.Errorf("<yellow>warning:</> %v\n", err)
	}
}

// rememberDevice saves the connected device's address. Best effort: a failure just means the next run scans by
// name again. An unreadable state file is replaced rather than left broken forever.
func rememberDevice(path string, found ble.Found) {
	st, err := state.Load(path)
	if err != nil {
		cout.Verbosef("<yellow>warning:</> %v; replacing it\n", err)
		if st, err = state.New(path); err != nil {
			return
		}
	}
	st.Remember(found.Name, found.Address)
	if err := st.Save(); err != nil {
		cout.Verbosef("<yellow>warning:</> remembering device: %v\n", err)
	}
}

func nameOr(name string) string {
	if name == "" {
		return "(unnamed)"
	}
	return name
}

// labelledPacket is a packet plus its console label.
type labelledPacket struct {
	label  string
	packet xggf.Packet
}

// lp pairs a packet with its own Describe() text as the label.
func lp(p xggf.Packet) labelledPacket {
	label := xggf.Describe(p)
	if label == "" {
		label = fmt.Sprintf("group %02X cmd %02X", p.Group(), p.Cmd())
	}
	return labelledPacket{label: label, packet: p}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
