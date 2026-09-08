package cli

import (
	"fmt"
	"strconv"

	"github.com/katbyte/acornvfd/lib/xggf"
	"github.com/spf13/cobra"
)

// lampCmd exposes the spectrum lamp (XGGF-XW25 / 8W25 / 8W30-PRO) commands decoded from the app. None of these
// have been tested against hardware; they are here so the whole app protocol is covered.
func lampCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lamp",
		Short: "spectrum lamp commands (XGGF-XW25/8W25/8W30-PRO) - decoded from the app, UNTESTED on hardware",
		Long: `Commands for the XGGF spectrum lamps, decoded from the same app but never tried against a real lamp.
They default to --name XGGF- so any lamp matches; pass --name XGGF-8W25 (etc.) or --address to be precise.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:           "power on|off",
		Short:         "turn the lamp on or off",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			on, err := parseOnOff(args[0])
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			f, err := lampFlags()
			if err != nil {
				return err
			}

			return f.sendPackets(labelledPacket{label: "lamp power " + args[0], packet: xggf.LampPower(on)})
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "mode <spectrum|ambient|effect> <index>",
		Short: "select a section and a mode within it (0-based, 10 spectrum modes in the app)",
		Long: `Sends the select-section packet, waits --gap (the app waits 80 ms), then the mode packet, mirroring
the app's tap handler. Mode indices are 0-based positions in the app's list for that section.`,
		Args:          cobra.ExactArgs(2),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			section, err := xggf.ParseLampSection(args[0])
			if err != nil {
				return err
			}
			idx, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("mode index must be a number: %w", err)
			}
			mode, err := xggf.LampMode(section, idx)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			f, err := lampFlags()
			if err != nil {
				return err
			}

			return f.sendPackets(
				labelledPacket{label: "lamp select " + section.Name, packet: xggf.LampSelectSection(section)},
				labelledPacket{label: fmt.Sprintf("lamp %s mode %d", section.Name, idx), packet: mode},
			)
		},
	})

	var hue, brightness int
	var changed string
	colour := &cobra.Command{
		Use:   "colour <spectrum|ambient>",
		Short: "set hue (0-360) and brightness (0-200) for a section",
		Long: `Sends the slider packet for a section. The app sends both values every time and tags which slider
moved (--changed colour|brightness); the effect section has no slider.`,
		Aliases:       []string{"color"},
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			section, err := xggf.ParseLampSection(args[0])
			if err != nil {
				return err
			}

			var update byte
			switch changed {
			case "colour", "color", "hue":
				update = xggf.LampUpdateColour
			case "brightness":
				update = xggf.LampUpdateBrightness
			default:
				return fmt.Errorf("--changed must be colour or brightness, got %q", changed)
			}

			p, err := xggf.LampSlider(section, hue, brightness, update)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			f, err := lampFlags()
			if err != nil {
				return err
			}

			return f.sendPackets(labelledPacket{label: fmt.Sprintf("lamp %s hue %d brightness %d", section.Name, hue, brightness), packet: p})
		},
	}
	colour.Flags().IntVar(&hue, "hue", 0, "hue in degrees, 0-360")
	colour.Flags().IntVar(&brightness, "brightness", 100, "brightness 0-200")
	colour.Flags().StringVar(&changed, "changed", "colour", "which slider moved: colour or brightness")
	cmd.AddCommand(colour)

	return cmd
}

// lampFlags widens the default --name from the clock to the whole XGGF family when the user did not set one.
func lampFlags() (*FlagData, error) {
	f, err := GetFlags()
	if err != nil {
		return nil, err
	}
	if f.Device.Name == "XGGF-1V48" && f.Device.Address == "" {
		f.Device.Name = "XGGF-"
	}
	return f, nil
}
