// Package xggf builds the 8-byte BLE command packets used by 橡果工坊 (Acorn Workshop) "XGGF" devices such as the
// 1V48 VFD clock and the spectrum lamps. The framing and every command were reverse-engineered from the vendor
// app, see PROTOCOL.md. Anything marked "guess" there is also marked in the doc comments here.
package xggf

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// Header is the fixed first byte of every packet.
	Header byte = 0xFF
	// PacketLen is the total packet length including the trailing checksum.
	PacketLen = 8
	// BodyLen is the number of bytes covered by the checksum (header + group + cmd + 4 params).
	BodyLen = PacketLen - 1
)

// Packet is a complete framed command: header, group, cmd, 4 parameter bytes, checksum.
type Packet [PacketLen]byte

// New frames a command. The checksum is the two's complement of the sum of the first 7 bytes so that all 8 bytes
// sum to 0 mod 256 (the app's va()/oa() helpers).
func New(group, cmd, p1, p2, p3, p4 byte) Packet {
	p := Packet{Header, group, cmd, p1, p2, p3, p4}
	p[PacketLen-1] = Checksum(p[:BodyLen])
	return p
}

// Checksum returns the byte that makes body plus the checksum sum to zero mod 256.
func Checksum(body []byte) byte {
	var sum byte
	for _, b := range body {
		sum += b
	}
	return -sum
}

// Bytes returns the packet as a slice for writing to a characteristic.
func (p Packet) Bytes() []byte {
	return p[:]
}

// Group returns the command group byte.
func (p Packet) Group() byte { return p[1] }

// Cmd returns the sub-command byte.
func (p Packet) Cmd() byte { return p[2] }

// Valid reports whether the header is present and the checksum is correct.
func (p Packet) Valid() bool {
	return p[0] == Header && Checksum(p[:]) == 0
}

// String renders the packet as space-separated upper-case hex, e.g. "FF 01 01 0F 17 00 00 DA".
func (p Packet) String() string {
	parts := make([]string, 0, PacketLen)
	for _, b := range p {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return strings.Join(parts, " ")
}

// ParseHex parses a raw packet from hex text. Separators (spaces, commas, colons, "0x" prefixes) are ignored.
// Seven bytes get the checksum appended; eight bytes are validated. Use ParseHexRaw to skip validation.
func ParseHex(s string) (Packet, error) {
	raw, err := ParseHexRaw(s)
	if err != nil {
		return Packet{}, err
	}

	var p Packet
	switch len(raw) {
	case BodyLen:
		copy(p[:], raw)
		p[PacketLen-1] = Checksum(raw)
	case PacketLen:
		copy(p[:], raw)
		if !p.Valid() {
			return Packet{}, fmt.Errorf("packet %s has a bad header or checksum (expected checksum %02X)", p, Checksum(raw[:BodyLen]))
		}
	default:
		return Packet{}, fmt.Errorf("expected %d or %d bytes, got %d", BodyLen, PacketLen, len(raw))
	}

	return p, nil
}

// ParseHexRaw parses arbitrary hex text into bytes without any framing rules.
func ParseHexRaw(s string) ([]byte, error) {
	clean := strings.NewReplacer(" ", "", ",", "", ":", "", "0x", "", "0X", "", "\t", "", "\n", "").Replace(s)
	if clean == "" {
		return nil, errors.New("no hex bytes given")
	}
	if len(clean)%2 != 0 {
		return nil, fmt.Errorf("odd number of hex digits in %q", s)
	}

	raw, err := hex.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("invalid hex %q: %w", s, err)
	}

	return raw, nil
}

func flag(on bool) byte {
	if on {
		return 1
	}
	return 0
}

func clampErr(name string, v, lo, hi int) error {
	if v < lo || v > hi {
		return fmt.Errorf("%s must be between %d and %d, got %d", name, lo, hi, v)
	}
	return nil
}
