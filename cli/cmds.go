// Package cli implements the acornvfd command line interface: the cobra commands, flag and config handling, and
// the scan / dump / send workflows built on lib/ble and lib/xggf.
package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	c "github.com/gookit/color"
	"github.com/katbyte/acornvfd/lib/ble"
	"github.com/katbyte/acornvfd/lib/cout"
	"github.com/katbyte/acornvfd/lib/version"
	"github.com/katbyte/acornvfd/lib/xggf"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Make builds the root command with every subcommand and flag registered.
func Make() (*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "acornvfd [command]",
		Short: "acornvfd controls 橡果工坊 (Acorn Workshop) XGGF Bluetooth LE devices, e.g. the 1V48 VFD clock",
		Long: `A small utility to control 橡果工坊 (Acorn Workshop) "XGGF" Bluetooth LE devices such as the 1V48
round-dial VFD clock without the vendor's WeChat mini-program. The protocol was reverse-engineered from the
vendor's Android app; see PROTOCOL.md for the packet format and what is verified versus guessed.

Devices are found by scanning for their advertised name (default prefix XGGF-1V48) or by --address.
Every command that talks to the device accepts --dry-run to print the packets instead of sending them.`,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			switch {
			case viper.GetBool("silent"):
				cout.Level = cout.VerbositySilent
				cmd.SilenceUsage = true
			case viper.GetBool("quiet"):
				cout.Level = cout.VerbosityQuiet
			case viper.GetBool("verbose"):
				cout.Level = cout.VerbosityVerbose
			default:
			}

			if viper.GetBool("uncoloured") {
				c.Enable = false
			}

			if viper.GetDuration("gap") < 0 {
				return errors.New("--gap cannot be negative")
			}

			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Println("Run \"acornvfd help\" for more information about available acornvfd commands.")
			return nil
		},
	}

	root.AddCommand(&cobra.Command{
		Use:           "version",
		Short:         "Print the version number of acornvfd",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("acornvfd " + version.Version)
		},
	})

	root.AddCommand(scanCmd())
	root.AddCommand(dumpCmd())
	root.AddCommand(listenCmd())
	root.AddCommand(rawCmd())
	addClockCommands(root)
	root.AddCommand(lampCmd())

	if err := configureFlags(root); err != nil {
		return nil, fmt.Errorf("unable to configure flags: %w", err)
	}

	return root, nil
}

func scanCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "discover nearby XGGF devices (or every BLE device with --all)",
		Long: `Scans for --scan-timeout and prints one line per device: address, RSSI and advertised name.
Only names starting with XGGF are shown unless --all is given. On macOS the address is a CoreBluetooth
UUID that is stable for this Mac but differs from the device's MAC.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			cout.Printf("scanning for <yellow>%s</>...\n", f.Device.ScanTimeout)

			n := 0
			if err := ble.Scan(f.Device.ScanTimeout, all, func(d ble.Found) bool {
				n++
				name := d.Name
				if name == "" {
					name = "<gray>(no name)</>"
				} else if strings.HasPrefix(strings.ToUpper(name), ble.NamePrefix) {
					name = "<green>" + name + "</>"
				}
				cout.Quietf("<cyan>%s</>  rssi <yellow>%4d</>  %s\n", d.Address, d.RSSI, name)
				return false
			}); err != nil {
				return err
			}

			if n == 0 {
				cout.Printf("<yellow>no devices found</>")
				if !all {
					cout.Printf(" (try <white>--all</> to list every BLE device)")
				}
				cout.Println()
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "show every BLE device, not only names starting with XGGF")

	return cmd
}

func dumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dump",
		Short: "connect and list every GATT service and characteristic on the device",
		Long: `Connects to the device and prints every service and characteristic, reading each value where the
device allows it. Use this to confirm the Nordic UART UUIDs from PROTOCOL.md before trusting the time command.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			conn, err := f.open()
			if err != nil {
				return err
			}
			defer closeConn(conn)

			conn.Dump(cout.Writer())

			return nil
		},
	}
}

func listenCmd() *cobra.Command {
	var (
		duration time.Duration
		sends    []string
	)

	cmd := &cobra.Command{
		Use:   "listen",
		Short: "subscribe to the notify characteristic and print anything the device sends",
		Long: `Connects, enables notifications on the UART TX characteristic (or --notify-char) and prints every
value received until --duration elapses (0 = until Ctrl-C) or Ctrl-C. The vendor app never listens to the clock,
so this is how to find out whether the device talks back at all. --send writes packets while listening, so a
reply to a specific command is caught in the same connection. With --dry-run the --send packets are only printed.`,
		Example:       "  acornvfd listen --duration 20s --send 'FF 05 04 01 00 00 00' --send 'FF 05 04 00 00 00 00'",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			f, err := GetFlags()
			if err != nil {
				return err
			}

			// parse every --send up front so a typo fails before we touch the radio; date/time are built at send time
			var probes []func() (xggf.Packet, error)
			for _, hexText := range sends {
				switch strings.ToLower(hexText) {
				case "date":
					probes = append(probes, func() (xggf.Packet, error) { return xggf.SetDate(time.Now()) })
				case "time":
					probes = append(probes, func() (xggf.Packet, error) { return xggf.SetTime(time.Now()), nil })
				default:
					p, perr := xggf.ParseHex(hexText)
					if perr != nil {
						return fmt.Errorf("--send %q: %w", hexText, perr)
					}
					probes = append(probes, func() (xggf.Packet, error) { return p, nil })
				}
			}

			if f.Send.DryRun {
				return f.withSender(func(s *sender) error {
					for _, build := range probes {
						p, perr := build()
						if perr != nil {
							return perr
						}
						if err := s.Packet(lp(p)); err != nil {
							return err
						}
					}
					return nil
				})
			}

			conn, err := f.connect()
			if err != nil {
				return err
			}
			defer closeConn(conn)

			if err := conn.Listen(func(b []byte) {
				cout.Quietf("<gray>%s</> <cyan>% X</>  %q\n", time.Now().Format("15:04:05.000"), b, ble.Printable(b))
			}); err != nil {
				return err
			}

			if duration > 0 {
				cout.Printf("listening on <cyan>%s</> for <yellow>%s</> (Ctrl-C to stop)...\n", conn.NotifyUUID(), duration)
			} else {
				cout.Printf("listening on <cyan>%s</> (Ctrl-C to stop)...\n", conn.NotifyUUID())
			}

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt)
			defer signal.Stop(sig)

			var deadline <-chan time.Time // nil = never fires
			if duration > 0 {
				deadline = time.After(duration)
			}

			// give the device a moment to volunteer anything, then send the probes with --gap between them
			if len(probes) > 0 {
				time.Sleep(time.Second)
			}
			for i, build := range probes {
				p, perr := build()
				if perr != nil {
					return perr
				}
				if i > 0 && f.Send.Gap > 0 {
					time.Sleep(f.Send.Gap)
				}
				cout.Quietf("<gray>%s</> <white>-> %-24s</> <cyan>%s</>\n", time.Now().Format("15:04:05.000"), lp(p).label, p)
				if err := conn.Write(p.Bytes(), !f.Send.NoResponse); err != nil {
					return err
				}
			}

			select {
			case <-deadline:
			case <-sig:
				cout.Println()
			}

			return nil
		},
	}

	cmd.Flags().DurationVar(&duration, "duration", 30*time.Second, "how long to listen (0 = until Ctrl-C)")
	cmd.Flags().StringArrayVar(&sends, "send", nil, "packet to send after subscribing: hex (7 or 8 bytes), or 'date' / 'time' for a sync packet built at send time; repeatable, sent --gap apart")

	return cmd
}

func rawCmd() *cobra.Command {
	var noChecksum bool

	cmd := &cobra.Command{
		Use:   "raw <hex bytes>...",
		Short: "send an arbitrary packet (7 bytes get the checksum appended, 8 are validated)",
		Long: `Sends raw bytes to the write characteristic, for probing commands the app does not expose.
Bytes are hex; spaces, commas, colons and 0x prefixes are ignored, so 'FF 05 02 03 00 00 00' and
'ff05020300 0000' are equivalent. Seven bytes get the two's-complement checksum appended; eight bytes
must already carry a correct checksum. --no-checksum sends whatever you typed, unframed.`,
		Example:       "  acornvfd raw FF 05 02 03 00 00 00    # brightness 3\n  acornvfd raw --dry-run FF 01 01 0F 17 00 00",
		Args:          cobra.MinimumNArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := GetFlags()
			if err != nil {
				return err
			}

			text := strings.Join(args, " ")
			var payload []byte
			label := "raw"
			if noChecksum {
				payload, err = xggf.ParseHexRaw(text)
			} else {
				var p xggf.Packet
				p, err = xggf.ParseHex(text)
				payload = p.Bytes()
				if d := xggf.Describe(p); d != "" {
					label = "raw (" + d + ")"
				}
			}
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			return f.sendBytes(label, payload)
		},
	}

	cmd.Flags().BoolVar(&noChecksum, "no-checksum", false, "send the bytes exactly as given, without framing or checksum")

	return cmd
}
