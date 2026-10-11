package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nevdelap/mdiff/internal/prototype"
	appversion "github.com/nevdelap/mdiff/internal/version"
)

func main() {
	flags := flag.NewFlagSet("mdiff", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configDir := flags.String("config-dir", "", "read Micro configuration from this existing directory")
	diffDebounce := flags.Int("diff-debounce", 0, "diff debounce in milliseconds (40-180)")
	version := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: mdiff [--config-dir DIR] [--diff-debounce MS] [--version]")
		_, _ = fmt.Fprintln(flags.Output(), "Run the disposable side-by-side Micro editing prototype.")
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *version {
		fmt.Println("mdiff (wrapping micro) " + appversion.Version)
		return
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "mdiff does not open files; launch it without file arguments")
		os.Exit(2)
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
			fmt.Fprintln(os.Stderr, "mdiff: --diff-debounce must be between 40 and 180 milliseconds")
			os.Exit(2)
		}
		debounceOverride = diffDebounce
	}
	if err := prototype.RunWithDebounce(*configDir, debounceOverride); err != nil {
		fmt.Fprintln(os.Stderr, "mdiff:", err)
		os.Exit(1)
	}
}
