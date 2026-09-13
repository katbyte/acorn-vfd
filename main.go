// Package main implements acornvfd, a CLI to control 橡果工坊 (Acorn Workshop) XGGF Bluetooth LE devices such as
// the 1V48 VFD clock without the vendor's WeChat mini-program.
package main

import (
	"os"

	"github.com/katbyte/go-kt/clog"

	"github.com/katbyte/acornvfd/cli"
	"github.com/katbyte/go-kt/cout"
)

func main() {
	// the log level comes from ACORNVFD_LOG; read it once here, before anything logs
	clog.SetLevelFromEnv("ACORNVFD_LOG")

	cmd, err := cli.Make()
	if err != nil {
		cout.Errorf("<red>acornvfd: building cmd:</> %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		cout.Errorf("<red>acornvfd:</> %v\n", err)
		os.Exit(1)
	}
}
