package xggf

import (
	"fmt"
	"strings"
)

// GroupLamp is the single command group used by the spectrum lamps (XGGF-XW25, 8W25, 8W30-PRO). Everything in
// this file is decoded from the app but has NOT been tested against hardware.
const (
	GroupLamp byte = 0xE0

	CmdLampSelectSection byte = 0x00 // FF E0 00 section
	CmdLampPower         byte = 0xFF // FF E0 FF flag

	LampHueMax        = 360
	LampBrightnessMax = 200

	// LampUpdateColour and LampUpdateBrightness are the "updateType" values the app sends with a slider packet.
	LampUpdateColour     byte = 1
	LampUpdateBrightness byte = 2
)

// LampSection is one of the three top-level lamp modes.
type LampSection struct {
	Name          string
	SelectValue   byte
	ModeCommand   byte
	SliderCommand byte // 0 = section has no colour/brightness slider
}

// Lamp sections as defined in the app (Dev2025_Spectrum.vue).
var (
	LampSpectrum = LampSection{Name: "spectrum", SelectValue: 0, ModeCommand: 1, SliderCommand: 2}
	LampAmbient  = LampSection{Name: "ambient", SelectValue: 1, ModeCommand: 4, SliderCommand: 5}
	LampEffect   = LampSection{Name: "effect", SelectValue: 2, ModeCommand: 6, SliderCommand: 0}

	LampSections = []LampSection{LampSpectrum, LampAmbient, LampEffect}
)

// ParseLampSection looks a section up by name (case-insensitive).
func ParseLampSection(name string) (LampSection, error) {
	for _, s := range LampSections {
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}

	return LampSection{}, fmt.Errorf("unknown lamp section %q (want spectrum, ambient or effect)", name)
}

// LampSelectSection switches the lamp to a section. The app sends this, waits 80 ms, then sends LampMode.
func LampSelectSection(s LampSection) Packet {
	return New(GroupLamp, CmdLampSelectSection, s.SelectValue, 0, 0, 0)
}

// LampMode picks a mode inside a section (0-based index into the section's mode list).
func LampMode(s LampSection, index int) (Packet, error) {
	if err := clampErr("mode index", index, 0, 255); err != nil {
		return Packet{}, err
	}

	return New(GroupLamp, s.ModeCommand, byte(index), 0, 0, 0), nil //nolint:gosec // G115: index is range-checked above
}

// LampSlider sends hue (0-360, big-endian 16-bit) and brightness (0-200) for a section, tagged with which one
// changed (LampUpdateColour or LampUpdateBrightness).
func LampSlider(s LampSection, hue, brightness int, updateType byte) (Packet, error) {
	if s.SliderCommand == 0 {
		return Packet{}, fmt.Errorf("lamp section %q has no colour/brightness slider", s.Name)
	}
	if err := clampErr("hue", hue, 0, LampHueMax); err != nil {
		return Packet{}, err
	}
	if err := clampErr("brightness", brightness, 0, LampBrightnessMax); err != nil {
		return Packet{}, err
	}

	return New(GroupLamp, s.SliderCommand, byte(hue>>8), byte(hue), byte(brightness), updateType), nil //nolint:gosec // G115: hue is 0-360 split into two bytes, brightness is range-checked above
}

// LampPower turns the lamp on or off.
func LampPower(on bool) Packet {
	return New(GroupLamp, CmdLampPower, flag(on), 0, 0, 0)
}
