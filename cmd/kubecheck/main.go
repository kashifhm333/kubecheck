package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/kubecheck/kubecheck/pkg/exitcode"
)

func main() {
	// Trap SIGINT and SIGTERM to enforce graceful shutdown per SAFE_EXECUTION_POLICY.md §6
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cli := NewCLI(os.Stdout, os.Stderr)

	exitChan := make(chan exitcode.ExitCode, 1)
	go func() {
		code := cli.Execute(os.Args[1:])
		exitChan <- code
	}()

	select {
	case <-ctx.Done():
		cli.Logger.Warn("Operation cancelled by user interrupt signal")
		os.Exit(exitcode.PartialScan.Int())
	case code := <-exitChan:
		os.Exit(code.Int())
	}
}
