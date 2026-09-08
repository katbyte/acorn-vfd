package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/katbyte/acornvfd/lib/xggf"
	"github.com/spf13/cobra"
)

// addClockCommands registers every control the vendor app exposes for the 1V48 VFD clock as a top-level command.
func addClockCommands(root *cobra.Command) {
	root.AddCommand(timeCmd())
	root.AddCommand(dateCmd())
	root.AddCommand(brightnessCmd())
	root.AddCommand(alarmCmd())
	root.AddCommand(displayCmd())
	root.AddCommand(toggleCmd("tick", "the per-second tick sound (秒钟提示音)", xggf.SecondTick))
	root.AddCommand(toggleCmd("transition", "the hourly transition animation (小时过渡动画)", xggf.HourTransition))
	root.AddCommand(toggleCmd("auto-sync", "the clock's 自动校时 (auto time sync) toggle", xggf.AutoTimeSync))
}

func timeCmd() *cobra.Command {
	var dateOnly, timeOnly bool

	cmd := &cobra.Command{
		Use:   "time [HH:MM[:SS]]",
		Short: "with no argument, sync date and time from this computer; with an argument, set the time",
		Long: `With no argument, sends the set-date packet followed by the set-time packet using this computer's
local wall-clock time, exactly as the app's two sync buttons do. The time is read after the connection is up so
the multi-second scan does not skew it. --date-only / --time-only limit the sync to one of the two.

With an HH:MM[:SS] argument, sets the clock's time to exactly that (24-hour) and nothing else.`,
		Example:       "  acornvfd time            # sync date + time now\n  acornvfd time 07:30",
		Aliases:       []string{"sync"},
		Args:          cobra.RangeArgs(0, 1),
		SilenceErrors: true,
		PreRunE: func(_ *cobra.Command, args []string) error {
			if dateOnly && timeOnly {
				return errors.New("--date-only and --time-only are mutually exclusive")
			}
			if len(args) == 1 && (dateOnly || timeOnly) {
				return errors.New("--date-only and --time-only apply only when syncing (no time argument)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			if len(args) == 1 {
				h, m, sec, perr := parseClockTime(args[0], true)
				if perr != nil {
					return perr
				}
				return f.sendPackets(lp(xggf.SetTime(time.Date(2000, 1, 1, h, m, sec, 0, time.UTC))))
			}

			return f.withSender(func(s *sender) error {
				if !timeOnly {
					p, derr := xggf.SetDate(time.Now())
					if derr != nil {
						return derr
					}
					if serr := s.Packet(lp(p)); serr != nil {
						return serr
					}
				}
				if !dateOnly {
					// re-read so the seconds are as fresh as possible after the date write and gap
					if serr := s.Packet(lp(xggf.SetTime(time.Now()))); serr != nil {
						return serr
					}
				}
				return nil
			})
		},
	}

	cmd.Flags().BoolVar(&dateOnly, "date-only", false, "when syncing, only send the date")
	cmd.Flags().BoolVar(&timeOnly, "time-only", false, "when syncing, only send the time")

	return cmd
}

func dateCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "date [YYYY-MM-DD]",
		Short:         "with no argument, set today's date from this computer; with an argument, set that date",
		Example:       "  acornvfd date            # set today's date\n  acornvfd date 2026-09-07",
		Args:          cobra.RangeArgs(0, 1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			t := time.Now()
			if len(args) == 1 {
				var err error
				if t, err = time.ParseInLocation("2006-01-02", args[0], time.Local); err != nil {
					return fmt.Errorf("date must be YYYY-MM-DD: %w", err)
				}
			}
			p, err := xggf.SetDate(t)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			return f.sendPackets(lp(p))
		},
	}
}

func brightnessCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "brightness <0-7|N%|auto|manual>",
		Short: "set manual brightness (0-7 or a percentage) or switch auto-brightness on/off",
		Long: `The app's slider maps 0-100 % onto levels 0-7 (round(pct/100*7)); pass either form.
'auto' turns automatic brightness on and 'manual' turns it off. Sending a level only switches the app's own
auto toggle off locally, so a level is sent on its own just like the app does; use 'brightness manual' first
if the clock seems to ignore the level.`,
		Example:       "  acornvfd brightness 5\n  acornvfd brightness 68%\n  acornvfd brightness auto",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				p   xggf.Packet
				err error
			)
			switch arg := strings.ToLower(strings.TrimSpace(args[0])); {
			case arg == "auto":
				p = xggf.AutoBrightness(true)
			case arg == "manual":
				p = xggf.AutoBrightness(false)
			case strings.HasSuffix(arg, "%"):
				pct, perr := strconv.Atoi(strings.TrimSuffix(arg, "%"))
				if perr != nil {
					return fmt.Errorf("percentage must be a whole number: %w", perr)
				}
				level, lerr := xggf.BrightnessFromPercent(pct)
				if lerr != nil {
					return lerr
				}
				p, err = xggf.Brightness(level)
			default:
				level, lerr := strconv.Atoi(arg)
				if lerr != nil {
					return fmt.Errorf("brightness must be 0-%d, a percentage, auto or manual", xggf.BrightnessMax)
				}
				p, err = xggf.Brightness(level)
			}
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			return f.sendPackets(lp(p))
		},
	}
}

func alarmCmd() *cobra.Command {
	var index, repeat int

	cmd := &cobra.Command{
		Use:   "alarm <HH:MM|on|off>",
		Short: "set the alarm time (24-hour) or turn the alarm on/off",
		Long: `Sets the alarm time, or enables/disables it. The app always sends alarm index 0 and repeat 0; --index and
--repeat exist because the packet has room for them, but any other value is untested (see PROTOCOL.md).`,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var packet xggf.Packet
			if on, err := parseOnOff(args[0]); err == nil {
				packet = xggf.AlarmEnable(on)
			} else {
				h, m, _, perr := parseClockTime(args[0], false)
				if perr != nil {
					return fmt.Errorf("alarm must be HH:MM, on or off: %w", perr)
				}
				p, aerr := xggf.AlarmSet(h, m, index, repeat)
				if aerr != nil {
					return aerr
				}
				packet = p
			}

			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			return f.sendPackets(lp(packet))
		},
	}

	cmd.Flags().IntVar(&index, "index", 0, "alarm slot (untested for anything but 0)")
	cmd.Flags().IntVar(&repeat, "repeat", 0, "repeat value (untested for anything but 0)")

	return cmd
}

func displayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "display analog|digital",
		Short: "switch the display mode (显示模式)",
		Long: `Sends the display-mode toggle. The app names the flag "analogClock" and defaults it to on; that
analog = round dial rendering and digital = plain digits is a guess until verified on the device.`,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var analog bool
			switch strings.ToLower(args[0]) {
			case "analog", "analogue", "on", "1":
				analog = true
			case "digital", "off", "0":
				analog = false
			default:
				return fmt.Errorf("display mode must be analog or digital, got %q", args[0])
			}

			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			return f.sendPackets(lp(xggf.DisplayMode(analog)))
		},
	}
}

// toggleCmd builds a plain on/off command around one of the flag-style packet builders.
func toggleCmd(name, what string, build func(bool) xggf.Packet) *cobra.Command {
	return &cobra.Command{
		Use:           name + " on|off",
		Short:         "turn " + what + " on or off",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			on, err := parseOnOff(args[0])
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			return f.sendPackets(lp(build(on)))
		},
	}
}

func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "true", "1", "yes", "enable", "enabled":
		return true, nil
	case "off", "false", "0", "no", "disable", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("expected on or off, got %q", s)
	}
}

// parseClockTime parses HH:MM or HH:MM:SS (24-hour). Seconds are only accepted when allowSeconds is set.
func parseClockTime(s string, allowSeconds bool) (hour, minute, second int, err error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 || (len(parts) == 3 && !allowSeconds) {
		if allowSeconds {
			return 0, 0, 0, fmt.Errorf("time must be HH:MM or HH:MM:SS, got %q", s)
		}
		return 0, 0, 0, fmt.Errorf("time must be HH:MM, got %q", s)
	}

	vals := make([]int, 3)
	limits := [3]int{23, 59, 59}
	names := [3]string{"hour", "minute", "second"}
	for i, part := range parts {
		v, cerr := strconv.Atoi(part)
		if cerr != nil || v < 0 || v > limits[i] {
			return 0, 0, 0, fmt.Errorf("%s must be 0-%d, got %q", names[i], limits[i], part)
		}
		vals[i] = v
	}

	return vals[0], vals[1], vals[2], nil
}
