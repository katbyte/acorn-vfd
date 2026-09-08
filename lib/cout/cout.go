// Package cout provides verbosity-levelled, coloured console output. Colour tags such as <red>...</> are rendered
// from the format string only, never from the arguments, so device names and other untrusted text print as-is.
package cout

import (
	"fmt"
	"io"
	"os"

	c "github.com/gookit/color"
)

// Verbosity levels (ordered from least to most output)
const (
	VerbositySilent = iota
	VerbosityQuiet
	VerbosityNormal
	VerbosityVerbose
)

// Level controls the output verbosity. Set before any output calls.
var Level = VerbosityNormal

// Writer returns the appropriate writer for normal output (os.Stdout or discard)
func Writer() io.Writer {
	if Level < VerbosityNormal {
		return io.Discard
	}
	return os.Stdout
}

// Sprintf renders the colour tags in format, then formats args into it.
func Sprintf(format string, args ...any) string {
	return fmt.Sprintf(c.ReplaceTag(format), args...)
}

// Printf prints normal output with colour support (suppressed in quiet and silent modes)
func Printf(format string, args ...any) {
	if Level < VerbosityNormal {
		return
	}
	fprintf(os.Stdout, format, args...)
}

// Println prints normal output (suppressed in quiet and silent modes)
func Println(args ...any) {
	if Level < VerbosityNormal {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, args...)
}

// Errorf prints an error to stderr in every mode except silent.
func Errorf(format string, args ...any) {
	if Level == VerbositySilent {
		return
	}
	fprintf(os.Stderr, format, args...)
}

// Quietf prints output in quiet mode and above; use it for the one line a script would parse.
func Quietf(format string, args ...any) {
	if Level < VerbosityQuiet {
		return
	}
	fprintf(os.Stdout, format, args...)
}

// Verbosef prints detailed output only when -v is set (suppressed at normal and below).
func Verbosef(format string, args ...any) {
	if Level < VerbosityVerbose {
		return
	}
	fprintf(os.Stdout, format, args...)
}

// console output: a failed write to the terminal is not actionable, so the error is deliberately dropped here
func fprintf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprint(w, Sprintf(format, args...))
}
