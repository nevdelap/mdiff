package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/nevdelap/mdiff/internal/prototype"
	appversion "github.com/nevdelap/mdiff/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("mdiff", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "read Micro configuration from this existing directory")
	diffDebounce := flags.Int("diff-debounce", 0, "diff debounce in milliseconds (40-180)")
	version := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: mdiff [--config-dir DIR] [--diff-debounce MS] [--version] [LEFT [RIGHT]]")
		_, _ = fmt.Fprintln(flags.Output(), "Edit and compare up to two files in side-by-side Micro panes.")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *version {
		_, _ = fmt.Fprintln(stdout, "mdiff (wrapping micro) "+appversion.Version)
		return 0
	}
	if flags.NArg() > 2 {
		_, _ = fmt.Fprintln(stderr, "mdiff accepts at most two file paths")
		return 2
	}
	var debounceOverride *int
	debounceSpecified := false
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == "diff-debounce" {
			debounceSpecified = true
		}
	})
	if debounceSpecified {
		if *diffDebounce < 40 || *diffDebounce > 180 {
			_, _ = fmt.Fprintln(stderr, "mdiff: --diff-debounce must be between 40 and 180 milliseconds")
			return 2
		}
		debounceOverride = diffDebounce
	}
	if err := prototype.RunWithFiles(*configDir, debounceOverride, flags.Args()...); err != nil {
		_, _ = fmt.Fprintln(stderr, "mdiff:", err)
		return 1
	}
	return 0
}
