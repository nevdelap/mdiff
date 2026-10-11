package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nevdelap/mdiff/internal/prototype"
)

func main() {
	flags := flag.NewFlagSet("mdiff", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configDir := flags.String("config-dir", "", "read Micro configuration from this existing directory")
	version := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: mdiff [--config-dir DIR] [--version]")
		_, _ = fmt.Fprintln(flags.Output(), "Run the disposable side-by-side Micro editing prototype.")
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *version {
		fmt.Println("mdiff prototype (Micro v2.0.15)")
		return
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "mdiff does not open files; launch it without file arguments")
		os.Exit(2)
	}
	if err := prototype.Run(*configDir); err != nil {
		fmt.Fprintln(os.Stderr, "mdiff:", err)
		os.Exit(1)
	}
}
