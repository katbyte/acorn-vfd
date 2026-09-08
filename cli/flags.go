package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/katbyte/acornvfd/lib/ble"
	"github.com/katbyte/acornvfd/lib/clog"
	"github.com/katbyte/acornvfd/lib/cout"
	"github.com/katbyte/acornvfd/lib/state"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// FlagData is the fully resolved configuration (flags, env vars and the .acornvfd config file merged by viper).
type FlagData struct {
	Device FlagsDevice `mapstructure:",squash"`
	Send   FlagsSend   `mapstructure:",squash"`

	StateFile  string `mapstructure:"state-file"`
	Verbose    bool   `mapstructure:"verbose"`
	Quiet      bool   `mapstructure:"quiet"`
	Silent     bool   `mapstructure:"silent"`
	Uncoloured bool   `mapstructure:"uncoloured"`
}

// FlagsDevice selects which device to talk to and which characteristics to use.
type FlagsDevice struct {
	Name        string        `mapstructure:"name"`
	Address     string        `mapstructure:"address"`
	ScanTimeout time.Duration `mapstructure:"scan-timeout"`
	Service     string        `mapstructure:"service"`
	WriteChar   string        `mapstructure:"write-char"`
	NotifyChar  string        `mapstructure:"notify-char"`
}

// FlagsSend controls how packets are written.
type FlagsSend struct {
	DryRun     bool          `mapstructure:"dry-run"`
	NoResponse bool          `mapstructure:"no-response"`
	Gap        time.Duration `mapstructure:"gap"`
}

// flagEnvMap is the full set of viper-managed flags and the env var each one can be set with ("" = flag only).
var flagEnvMap = map[string]string{
	"name":         "ACORNVFD_NAME",
	"address":      "ACORNVFD_ADDRESS",
	"scan-timeout": "ACORNVFD_SCAN_TIMEOUT",
	"service":      "ACORNVFD_SERVICE",
	"write-char":   "ACORNVFD_WRITE_CHAR",
	"notify-char":  "ACORNVFD_NOTIFY_CHAR",
	"gap":          "ACORNVFD_GAP",
	"no-response":  "ACORNVFD_NO_RESPONSE",
	"dry-run":      "",
	"state-file":   "ACORNVFD_STATE_FILE",
	"verbose":      "",
	"quiet":        "ACORNVFD_QUIET",
	"silent":       "ACORNVFD_SILENT",
	"uncoloured":   "ACORNVFD_UNCOLOURED",
}

func configureFlags(root *cobra.Command) error {
	pflags := root.PersistentFlags()

	// Device selection (FlagsDevice)
	pflags.StringP("name", "n", "XGGF-1V48", "advertised name prefix of the device to use (case-insensitive)")
	pflags.StringP("address", "a", "", "exact device address from scan (MAC on Linux/Windows, UUID on macOS); overrides --name")
	pflags.Duration("scan-timeout", ble.DefaultScanTimeout, "how long to scan for the device before giving up")
	pflags.String("service", "", "service UUID to use instead of auto-detecting the Nordic UART service (needs --write-char)")
	pflags.String("write-char", "", "characteristic UUID to write commands to (needs --service)")
	pflags.String("notify-char", "", "characteristic UUID to subscribe to for listen (optional with --service)")

	// Sending (FlagsSend)
	pflags.Bool("dry-run", false, "print the packets that would be sent without touching Bluetooth")
	pflags.Bool("no-response", false, "use write-without-response instead of write-with-response (the app uses with-response)")
	pflags.Duration("gap", 100*time.Millisecond, "pause between consecutive packets")

	pflags.String("state-file", "", "where to remember the clock's address (default <user config dir>/acornvfd/device.json)")

	// Output
	pflags.BoolP("verbose", "v", false, "show scan/connect progress and every packet")
	pflags.BoolP("quiet", "q", false, "only print the essential result line")
	pflags.Bool("silent", false, "suppress all output")
	pflags.BoolP("uncoloured", "u", false, "disable coloured output")

	for name, env := range flagEnvMap {
		if err := viper.BindPFlag(name, pflags.Lookup(name)); err != nil {
			return fmt.Errorf("error binding '%s' flag: %w", name, err)
		}

		if env != "" {
			if err := viper.BindEnv(name, env); err != nil {
				return fmt.Errorf("error binding '%s' to env '%s': %w", name, env, err)
			}
		}
	}

	viper.SetConfigName(".acornvfd")
	viper.SetConfigType("env")
	if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(home)
	}
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			clog.Log.Errorf("Error reading config file: %v", err)
		}
	}

	return nil
}

// GetFlags returns the fully populated FlagData. We unmarshal from viper instead of reading pflags directly because
// pflags only parse command-line arguments; viper merges environment variables and the config file on top.
func GetFlags() (*FlagData, error) {
	var f FlagData
	if err := viper.Unmarshal(&f); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	return &f, nil
}

// DefaultName is the built-in device name prefix; used to tell whether the user overrode --name.
const DefaultName = "XGGF-1V48"

// BLEOptions converts the device flags into ble.Options, routing progress output through cout at verbose level.
// When the user gave neither --address nor a custom --name, it reuses the address remembered from the last
// successful connection: the clock frequently advertises with no name, so name matching alone is unreliable, but
// its address is stable per host and reconnecting by it just works.
func (f *FlagData) BLEOptions() ble.Options {
	address := f.Device.Address
	if address == "" && f.Device.Name == DefaultName {
		if st, err := state.Load(f.StateFile); err == nil && st.Address != "" {
			address = st.Address
			cout.Verbosef("<gray>using remembered address %s (%s); pass --address or --name to override</>\n", address, st.Device)
		}
	}

	return ble.Options{
		Name:        f.Device.Name,
		Address:     address,
		ScanTimeout: f.Device.ScanTimeout,
		Service:     f.Device.Service,
		WriteChar:   f.Device.WriteChar,
		NotifyChar:  f.Device.NotifyChar,
		Logf: func(format string, args ...any) {
			cout.Verbosef("<gray>"+format+"</>\n", args...)
		},
	}
}
