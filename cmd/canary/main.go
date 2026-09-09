package main

import (
	"fmt"
	"os"

	"github.com/tolvi-labs/canary/internal/cmdaudit"
	"github.com/tolvi-labs/canary/internal/cmdinit"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println("canary " + version)
	case "init":
		os.Exit(cmdinit.Run(os.Args[2:]))
	case "audit":
		os.Exit(cmdaudit.Run(os.Args[2:]))
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "canary: unknown command %q\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println(`canary — vault-informed CI test selection

  Usage:
    canary version
    canary init [--audit]
    canary audit
    canary check --base <ref> --head <ref> [--gate pr|merge|release]   (not yet available)
    canary refresh                                                     (not yet available)
    canary hook install|uninstall                                      (not yet available)`)
}
