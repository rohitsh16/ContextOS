package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"contextos/internal/hook"
)

func main() {
	fs := flag.NewFlagSet("ctx-hook", flag.ContinueOnError)
	agent := fs.String("agent", "unknown", "agent name")
	event := fs.String("event", "unknown", "event name")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := hook.Handle(*agent, *event, raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, string(out))
}
