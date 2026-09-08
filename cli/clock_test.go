package cli

import "testing"

func TestParseClockTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in           string
		allowSeconds bool
		h, m, s      int
		wantErr      bool
	}{
		{in: "07:30", h: 7, m: 30},
		{in: " 7:5 ", h: 7, m: 5},
		{in: "23:59:59", allowSeconds: true, h: 23, m: 59, s: 59},
		{in: "23:59:59", wantErr: true}, // seconds not allowed for alarms
		{in: "24:00", wantErr: true},
		{in: "12:60", wantErr: true},
		{in: "12:30:60", allowSeconds: true, wantErr: true},
		{in: "-1:00", wantErr: true},
		{in: "1230", wantErr: true},
		{in: "a:b", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			h, m, s, err := parseClockTime(tt.in, tt.allowSeconds)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %d:%d:%d", h, m, s)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if h != tt.h || m != tt.m || s != tt.s {
				t.Fatalf("got %d:%d:%d, want %d:%d:%d", h, m, s, tt.h, tt.m, tt.s)
			}
		})
	}
}

func TestParseOnOff(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"on", "ON", " true ", "1", "yes", "enable", "enabled"} {
		if on, err := parseOnOff(in); err != nil || !on {
			t.Fatalf("parseOnOff(%q) = %v, %v; want true", in, on, err)
		}
	}
	for _, in := range []string{"off", "False", "0", "no", "disable", "disabled"} {
		if on, err := parseOnOff(in); err != nil || on {
			t.Fatalf("parseOnOff(%q) = %v, %v; want false", in, on, err)
		}
	}
	for _, in := range []string{"", "maybe", "2", "on off"} {
		if _, err := parseOnOff(in); err == nil {
			t.Fatalf("parseOnOff(%q) accepted", in)
		}
	}
}

func TestParseBrightness(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"5":      "FF 05 02 05 00 00 00 F5",
		"0":      "FF 05 02 00 00 00 00 FA",
		"7":      "FF 05 02 07 00 00 00 F3",
		"68%":    "FF 05 02 05 00 00 00 F5",
		"100%":   "FF 05 02 07 00 00 00 F3",
		"auto":   "FF 05 01 00 00 00 00 FB",
		"MANUAL": "FF 05 00 00 00 00 00 FC",
	}
	for in, want := range tests {
		p, err := parseBrightness(in)
		if err != nil {
			t.Fatalf("parseBrightness(%q): %v", in, err)
		}
		if p.String() != want {
			t.Fatalf("parseBrightness(%q) = %s, want %s", in, p, want)
		}
	}

	for _, in := range []string{"8", "-1", "101%", "x%", "bright", ""} {
		if _, err := parseBrightness(in); err == nil {
			t.Fatalf("parseBrightness(%q) accepted", in)
		}
	}
}
