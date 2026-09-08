// Package main implements acornvfd, a CLI to control 橡果工坊 (Acorn Workshop) XGGF Bluetooth LE devices such as
// the 1V48 VFD clock without the vendor's WeChat mini-program.
package main

import (
	"os"

	c "github.com/gookit/color"
	"github.com/katbyte/acornvfd/cli"
	"github.com/katbyte/acornvfd/lib/clog"
)

func main() {
	cmd, err := cli.Make()
	if err != nil {
		clog.Log.Error(c.Sprintf("<red>acornvfd: building cmd</> %v", err))

		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		clog.Log.Error(c.Sprintf("<red>acornvfd:</> %v", err))

		os.Exit(1)
	}

	os.Exit(0)
}
