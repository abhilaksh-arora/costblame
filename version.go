package main

import (
	"fmt"
	"runtime/debug"
)

// version is stamped at build time via -ldflags "-X main.version=vX.Y.Z" (see
// Makefile). `go install ...@latest` doesn't run our Makefile, so it falls
// back to the module's pseudo-version from the build info, then finally to
// "dev" for a plain `go build` with no version info available at all.
var version = "dev"

func resolvedVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func runVersion(args []string) {
	fmt.Println("costblame " + resolvedVersion())
}
