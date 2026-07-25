package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// runUpdate re-runs the same install.sh a fresh `curl | sh` install would use,
// so there's exactly one place (install.sh) that knows how to fetch and place
// a costblame binary. Windows has no self-update path here since install.sh
// is POSIX shell; those users grab the .exe from the releases page directly.
func runUpdate(args []string) {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "costblame: self-update isn't available on Windows yet.")
		fmt.Fprintln(os.Stderr, "Download the latest costblame-windows.zip from:")
		fmt.Fprintln(os.Stderr, "  https://github.com/abhilaksh-arora/costblame/releases/latest")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "costblame: currently %s, checking for updates...\n", resolvedVersion())

	cmd := exec.Command("sh", "-c",
		"curl -fsSL https://raw.githubusercontent.com/abhilaksh-arora/costblame/main/install.sh | sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fatal("update failed: %v", err)
	}
}
