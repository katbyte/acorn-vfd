package xggf

import (
	"fmt"
	"math"
	"time"
)

// Command groups and sub-commands for the 1V48 VFD clock (app page Dev2025_FluorescentClock).
const (
	GroupDateTime byte = 0x01
	GroupAlarm    byte = 0x04
	GroupDisplay  byte = 0x05

	CmdSetDate byte = 0x00 // FF 01 00 YY MM DD 00
	CmdSetTime byte = 0x01 // FF 01 01 hh mm ss 00

	CmdAlarmSet byte = 0x02 // FF 04 02 idx hh mm repeat; alarm enable/disable puts the flag in the cmd byte

	CmdBrightness     byte = 0x02 // FF 05 02 level 00 00 00; auto-brightness on/off puts the flag in the cmd byte
	CmdDisplayMode    byte = 0x03 // FF 05 03 flag
	CmdSecondTick     byte = 0x04 // FF 05 04 flag
	CmdHourTransition byte = 0x05 // FF 05 05 flag
	CmdAutoTimeSync   byte = 0x06 // FF 05 06 flag

	// BrightnessMax is the highest manual brightness level; the app maps its 0-100 % slider onto 0-7.
	BrightnessMax = 7

	// YearMin and YearMax bound the date. The app clamps its picker to 2000-2555 but sends year-2000 as a single
	// byte, so anything past 2255 would wrap; we refuse it instead.
	YearMin = 2000
	YearMax = YearMin + 255
)

// SetDate builds the set-date packet from t. The year is sent as an offset from 2000.
func SetDate(t time.Time) (Packet, error) {
	if err := clampErr("year", t.Year(), YearMin, YearMax); err != nil {
		return Packet{}, err
	}

	return New(GroupDateTime, CmdSetDate, byte(t.Year()-YearMin), byte(t.Month()), byte(t.Day()), 0), nil //nolint:gosec // G115: year is range-checked above, month/day are 1-12/1-31
}

// SetTime builds the set-time packet from the wall-clock fields of t (24-hour, the clock has no time-zone notion).
func SetTime(t time.Time) Packet {
	return New(GroupDateTime, CmdSetTime, byte(t.Hour()), byte(t.Minute()), byte(t.Second()), 0) //nolint:gosec // G115: time.Time fields are 0-23/0-59/0-59
}

// AlarmSet builds the set-alarm packet. The app only ever sends index 0 and repeat 0; other values are untested
// (guess: index selects one of several alarms, repeat is a weekday mask or count).
func AlarmSet(hour, minute, index, repeat int) (Packet, error) {
	if err := clampErr("hour", hour, 0, 23); err != nil {
		return Packet{}, err
	}
	if err := clampErr("minute", minute, 0, 59); err != nil {
		return Packet{}, err
	}
	if err := clampErr("index", index, 0, math.MaxUint8); err != nil {
		return Packet{}, err
	}
	if err := clampErr("repeat", repeat, 0, math.MaxUint8); err != nil {
		return Packet{}, err
	}

	return New(GroupAlarm, CmdAlarmSet, byte(index), byte(hour), byte(minute), byte(repeat)), nil //nolint:gosec // G115: all four are range-checked above
}

// AlarmEnable turns the alarm on or off. Note the flag travels in the cmd byte: FF 04 <0|1> 00 00 00 00.
func AlarmEnable(on bool) Packet {
	return New(GroupAlarm, flag(on), 0, 0, 0, 0)
}

// AutoBrightness turns automatic brightness on or off. The flag travels in the cmd byte: FF 05 <0|1> 00 00 00 00.
func AutoBrightness(on bool) Packet {
	return New(GroupDisplay, flag(on), 0, 0, 0, 0)
}

// Brightness sets a manual brightness level 0..BrightnessMax. The app flips its own auto-brightness toggle off when
// sending this but does not send an explicit auto-off packet first.
func Brightness(level int) (Packet, error) {
	if err := clampErr("brightness", level, 0, BrightnessMax); err != nil {
		return Packet{}, err
	}

	return New(GroupDisplay, CmdBrightness, byte(level), 0, 0, 0), nil //nolint:gosec // G115: level is range-checked above
}

// BrightnessFromPercent maps a 0-100 slider value onto a level exactly like the app: round(pct/100*7).
func BrightnessFromPercent(pct int) (int, error) {
	if err := clampErr("percent", pct, 0, 100); err != nil {
		return 0, err
	}

	return int(math.Round(float64(pct) / 100 * BrightnessMax)), nil
}

// DisplayMode selects the display mode. The app calls the toggle 显示模式 ("display mode") and names the bool
// analogClock, default on. Guess: true = round analogue dial, false = digital.
func DisplayMode(analog bool) Packet {
	return New(GroupDisplay, CmdDisplayMode, flag(analog), 0, 0, 0)
}

// SecondTick turns the per-second tick sound (秒钟提示音) on or off.
func SecondTick(on bool) Packet {
	return New(GroupDisplay, CmdSecondTick, flag(on), 0, 0, 0)
}

// HourTransition turns the hourly transition animation (小时过渡动画) on or off.
func HourTransition(on bool) Packet {
	return New(GroupDisplay, CmdHourTransition, flag(on), 0, 0, 0)
}

// AutoTimeSync turns the 自动校时 ("auto time sync") toggle on or off. What the firmware does with it is unknown;
// the app never pushes time in response to anything.
func AutoTimeSync(on bool) Packet {
	return New(GroupDisplay, CmdAutoTimeSync, flag(on), 0, 0, 0)
}

// Describe returns a short human label for a known clock packet, or "" if it is not a recognised clock command.
func Describe(p Packet) string {
	switch p.Group() {
	case GroupDateTime:
		switch p.Cmd() {
		case CmdSetDate:
			return fmt.Sprintf("set date %04d-%02d-%02d", YearMin+int(p[3]), p[4], p[5])
		case CmdSetTime:
			return fmt.Sprintf("set time %02d:%02d:%02d", p[3], p[4], p[5])
		}
	case GroupAlarm:
		switch p.Cmd() {
		case 0, 1:
			return "alarm " + onOff(p.Cmd() == 1)
		case CmdAlarmSet:
			return fmt.Sprintf("set alarm %d to %02d:%02d (repeat %d)", p[3], p[4], p[5], p[6])
		}
	case GroupDisplay:
		switch p.Cmd() {
		case 0, 1:
			return "auto-brightness " + onOff(p.Cmd() == 1)
		case CmdBrightness:
			return fmt.Sprintf("brightness %d/%d", p[3], BrightnessMax)
		case CmdDisplayMode:
			if p[3] == 1 {
				return "display mode analog"
			}
			return "display mode digital"
		case CmdSecondTick:
			return "second tick " + onOff(p[3] == 1)
		case CmdHourTransition:
			return "hour transition " + onOff(p[3] == 1)
		case CmdAutoTimeSync:
			return "auto time sync " + onOff(p[3] == 1)
		}
	default:
	}

	return ""
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
