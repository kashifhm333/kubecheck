package main

import (
	"fmt"
	"io"
)

// Version information populated at build time via -ldflags.
var (
	EngineVersion = "1.0.0-dev"
	BundleVersion = "2026.10.1"
	GitCommit     = "HEAD"
	BuildDate     = "2026-10-31"
	SupportedK8s  = "v1.24, v1.25, v1.26, v1.27, v1.28, v1.29, v1.30, v1.31"
)

// PrintVersion outputs the engine and knowledge bundle version details.
func PrintVersion(w io.Writer) {
	fmt.Fprintf(w, "KubeCheck Engine:            %s\n", EngineVersion)
	fmt.Fprintf(w, "Default Knowledge Bundle:     %s\n", BundleVersion)
	fmt.Fprintf(w, "Supported Kubernetes APIs:    %s\n", SupportedK8s)
	fmt.Fprintf(w, "Git Commit:                   %s\n", GitCommit)
	fmt.Fprintf(w, "Build Date:                   %s\n", BuildDate)
}
