package xggf_test

import (
	"testing"
	"time"

	"github.com/katbyte/acornvfd/lib/xggf"
)

func TestChecksum(t *testing.T) {
	t.Parallel()

	// worked example from PROTOCOL.md: set time 15:23:00
	if got := xggf.Checksum([]byte{0xFF, 0x01, 0x01, 0x0F, 0x17, 0x00, 0x00}); got != 0xD9 {
		t.Fatalf("Checksum = %02X, want D9", got)
	}

	// mirrors the app's va(): every packet sums to 0 mod 256
	for g := range 256 {
		p := xggf.New(byte(g), byte(255-g), 0x12, 0x34, 0x56, 0x78)
		if !p.Valid() {
			t.Fatalf("packet %s does not sum to zero", p)
		}
	}
}

func TestNewAndString(t *testing.T) {
	t.Parallel()

	p := xggf.New(0x01, 0x01, 15, 23, 0, 0)
	if want := "FF 01 01 0F 17 00 00 D9"; p.String() != want {
		t.Fatalf("String = %q, want %q", p.String(), want)
	}
	if p.Group() != 0x01 || p.Cmd() != 0x01 {
		t.Fatalf("group/cmd = %02X/%02X", p.Group(), p.Cmd())
	}
	if len(p.Bytes()) != xggf.PacketLen {
		t.Fatalf("Bytes len = %d", len(p.Bytes()))
	}
}

func TestParseHex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "seven bytes get checksum", in: "FF 01 01 0F 17 00 00", want: "FF 01 01 0F 17 00 00 D9"},
		{name: "eight valid bytes", in: "ff0101 0f 17 0000d9", want: "FF 01 01 0F 17 00 00 D9"},
		{name: "0x prefixes and commas", in: "0xFF,0x01,0x01,0x0F,0x17,0x00,0x00", want: "FF 01 01 0F 17 00 00 D9"},
		{name: "bad checksum", in: "FF 01 01 0F 17 00 00 00", wantErr: true},
		{name: "bad header", in: "FE 01 01 0F 17 00 00", wantErr: true},
		{name: "wrong length", in: "FF 01", wantErr: true},
		{name: "odd digits", in: "FF 0", wantErr: true},
		{name: "not hex", in: "FF ZZ 01 0F 17 00 00", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := xggf.ParseHex(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want != "" && got.String() != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestClockPackets(t *testing.T) {
	t.Parallel()

	date := time.Date(2026, time.September, 7, 15, 23, 0, 0, time.Local)

	tests := []struct {
		name string
		got  func() (xggf.Packet, error)
		want string
	}{
		{"set date", func() (xggf.Packet, error) { return xggf.SetDate(date) }, "FF 01 00 1A 09 07 00 D6"},
		{"set time", func() (xggf.Packet, error) { return xggf.SetTime(date), nil }, "FF 01 01 0F 17 00 00 D9"},
		{"alarm set 08:00", func() (xggf.Packet, error) { return xggf.AlarmSet(8, 0, 0, 0) }, "FF 04 02 00 08 00 00 F3"},
		{"alarm on", func() (xggf.Packet, error) { return xggf.AlarmEnable(true), nil }, "FF 04 01 00 00 00 00 FC"},
		{"alarm off", func() (xggf.Packet, error) { return xggf.AlarmEnable(false), nil }, "FF 04 00 00 00 00 00 FD"},
		{"auto brightness on", func() (xggf.Packet, error) { return xggf.AutoBrightness(true), nil }, "FF 05 01 00 00 00 00 FB"},
		{"brightness 5", func() (xggf.Packet, error) { return xggf.Brightness(5) }, "FF 05 02 05 00 00 00 F5"},
		{"display analog", func() (xggf.Packet, error) { return xggf.DisplayMode(true), nil }, "FF 05 03 01 00 00 00 F8"},
		{"second tick off", func() (xggf.Packet, error) { return xggf.SecondTick(false), nil }, "FF 05 04 00 00 00 00 F8"},
		{"hour transition on", func() (xggf.Packet, error) { return xggf.HourTransition(true), nil }, "FF 05 05 01 00 00 00 F6"},
		{"auto time sync on", func() (xggf.Packet, error) { return xggf.AutoTimeSync(true), nil }, "FF 05 06 01 00 00 00 F5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := tt.got()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.String() != tt.want {
				t.Fatalf("got %s, want %s", p, tt.want)
			}
			if !p.Valid() {
				t.Fatalf("packet %s is not valid", p)
			}
			if xggf.Describe(p) == "" {
				t.Fatalf("Describe(%s) is empty", p)
			}
		})
	}
}

func TestClockValidation(t *testing.T) {
	t.Parallel()

	if _, err := xggf.SetDate(time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected year < 2000 to fail")
	}
	if _, err := xggf.SetDate(time.Date(xggf.YearMax, 12, 31, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("expected year %d to be accepted: %v", xggf.YearMax, err)
	}
	if _, err := xggf.SetDate(time.Date(xggf.YearMax+1, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatalf("expected year %d to fail (year-2000 must fit one byte)", xggf.YearMax+1)
	}
	if _, err := xggf.Brightness(8); err == nil {
		t.Fatal("expected brightness 8 to fail")
	}
	if _, err := xggf.Brightness(-1); err == nil {
		t.Fatal("expected brightness -1 to fail")
	}
	if _, err := xggf.AlarmSet(24, 0, 0, 0); err == nil {
		t.Fatal("expected hour 24 to fail")
	}
	if _, err := xggf.AlarmSet(0, 60, 0, 0); err == nil {
		t.Fatal("expected minute 60 to fail")
	}
}

func TestBrightnessFromPercent(t *testing.T) {
	t.Parallel()

	// the app's default slider position is 68 % which it turns into level 5
	tests := map[int]int{0: 0, 7: 0, 8: 1, 50: 4, 68: 5, 100: 7}
	for pct, want := range tests {
		got, err := xggf.BrightnessFromPercent(pct)
		if err != nil {
			t.Fatalf("%d%%: %v", pct, err)
		}
		if got != want {
			t.Fatalf("%d%% -> %d, want %d", pct, got, want)
		}
	}
	if _, err := xggf.BrightnessFromPercent(101); err == nil {
		t.Fatal("expected 101% to fail")
	}
}

func TestLampPackets(t *testing.T) {
	t.Parallel()

	if p := xggf.LampSelectSection(xggf.LampAmbient); p.String() != "FF E0 00 01 00 00 00 20" {
		t.Fatalf("select ambient = %s", p)
	}
	p, err := xggf.LampMode(xggf.LampSpectrum, 3)
	if err != nil || p.String() != "FF E0 01 03 00 00 00 1D" {
		t.Fatalf("spectrum mode 3 = %s, %v", p, err)
	}
	p, err = xggf.LampSlider(xggf.LampSpectrum, 300, 150, xggf.LampUpdateColour)
	if err != nil || p.String() != "FF E0 02 01 2C 96 01 5B" {
		t.Fatalf("slider = %s, %v", p, err)
	}
	if _, err := xggf.LampSlider(xggf.LampEffect, 0, 0, xggf.LampUpdateColour); err == nil {
		t.Fatal("expected effect section to reject slider")
	}
	if _, err := xggf.LampSlider(xggf.LampSpectrum, 361, 0, xggf.LampUpdateColour); err == nil {
		t.Fatal("expected hue 361 to fail")
	}
	if p := xggf.LampPower(true); p.String() != "FF E0 FF 01 00 00 00 21" {
		t.Fatalf("power on = %s", p)
	}
	if _, err := xggf.ParseLampSection("nope"); err == nil {
		t.Fatal("expected unknown section to fail")
	}
	if s, err := xggf.ParseLampSection("AMBIENT"); err != nil || s.Name != "ambient" {
		t.Fatalf("ParseLampSection = %v, %v", s, err)
	}
}
