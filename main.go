// Package main implements acornvfd, a CLI to control 橡果工坊 (Acorn Workshop) XGGF Bluetooth LE devices such as
// the 1V48 VFD clock without the vendor's WeChat mini-program.
package main

import (
	"os"

	"github.com/katbyte/acornvfd/cli"
	"github.com/katbyte/acornvfd/lib/cout"
)

func main() {
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
