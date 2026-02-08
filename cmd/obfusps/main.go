package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/benzoXdev/obfusps/internal/engine"
)

func main() {
	// Clean exit on Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Fprintln(os.Stderr, "\n\033[33mInterrupted.\033[0m")
		os.Exit(130)
	}()

	doubleClick := len(os.Args) == 1
	if doubleClick {
		os.Args = append(os.Args, "-h")
	}
	opts, helpOnly := engine.ParseFlags()
	if helpOnly {
		if doubleClick && runtime.GOOS == "windows" {
			fmt.Fprint(os.Stdout, "\nPress Enter to exit...")
			bufio.NewScanner(os.Stdin).Scan()
		}
		os.Exit(0)
	}
	start := time.Now()
	if err := engine.Run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mError:\033[0m %v\n", err)
		if hint := engine.ErrorHint(err); hint != "" {
			fmt.Fprintf(os.Stderr, "\033[90mHint:\033[0m %s\n", hint)
		}
		os.Exit(1)
	}
	if !opts.Quiet {
		fmt.Fprintf(os.Stderr, "\033[90mDone in %s\033[0m\n", time.Since(start).Round(time.Millisecond))
	}
}
