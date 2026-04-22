package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/pizzasaurusrex/homecast/internal/version"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("homecast", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintln(stdout, version.String())
		return nil
	}
	fmt.Fprintln(stderr, "homecast: no command given (try --version)")
	return errors.New("no command")
}
