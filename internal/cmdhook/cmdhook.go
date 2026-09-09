package cmdhook

import (
	"fmt"
	"os"
	"path/filepath"
)

const shim = "#!/usr/bin/env sh\n" +
	"# canary post-commit hook — installed by `canary hook install`.\n" +
	"# Incrementally refreshes the global coverage manifest after each commit.\n" +
	"command -v canary >/dev/null 2>&1 || exit 0\n" +
	"canary refresh\n"

// Run implements `canary hook install|uninstall`.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "canary hook: expected install or uninstall")
		return 2
	}
	switch args[0] {
	case "install":
		force := len(args) > 1 && args[1] == "--force"
		if err := install(".", force); err != nil {
			fmt.Fprintf(os.Stderr, "canary hook install: %v\n", err)
			return 1
		}
		fmt.Println("✓ Installed post-commit hook")
		return 0
	case "uninstall":
		if err := uninstall("."); err != nil {
			fmt.Fprintf(os.Stderr, "canary hook uninstall: %v\n", err)
			return 1
		}
		fmt.Println("✓ Removed post-commit hook")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "canary hook: unknown subcommand %q\n", args[0])
		return 2
	}
}

func hookPath(repoDir string) string {
	return filepath.Join(repoDir, ".git", "hooks", "post-commit")
}

// install writes the post-commit shim to repoDir's .git/hooks/post-commit.
// It refuses to overwrite an existing hook unless force is true.
func install(repoDir string, force bool) error {
	path := hookPath(repoDir)
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(shim), 0755); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// uninstall removes the post-commit shim if it was installed by this tool.
func uninstall(repoDir string) error {
	path := hookPath(repoDir)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if string(content) != shim {
		return fmt.Errorf("%s was not installed by canary (leaving it in place)", path)
	}
	return os.Remove(path)
}
