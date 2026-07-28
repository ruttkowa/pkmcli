package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pkm/internal/index"
	"pkm/internal/installer"
	"pkm/internal/tui"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	install := flag.Bool("install", false, "install this pkm binary for the current user")
	installDir := flag.String("install-dir", "", "binary directory used with --install (default ~/.local/bin)")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: pkm [flags] [vault-directory]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *install {
		destination := ""
		if *installDir != "" {
			destination = filepath.Join(*installDir, "pkm")
		}
		if err := installer.Install(installer.Options{
			Destination: destination,
			Shell:       os.Getenv("SHELL"),
			Path:        os.Getenv("PATH"),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "pkm: install: %v\n", err)
			os.Exit(1)
		}
		return
	}

	vaultPath := "."
	if flag.NArg() > 0 {
		vaultPath = flag.Arg(0)
	} else {
		fmt.Fprint(os.Stderr, "Vault path [.]: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			if t := strings.TrimSpace(scanner.Text()); t != "" {
				vaultPath = t
			}
		}
	}

	v, err := vault.Open(vaultPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pkm: open vault: %v\n", err)
		os.Exit(1)
	}

	idx, err := index.Open(filepath.Join(v.Root, ".pkm"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "pkm: open index: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()

	m := tui.New(v, idx)

	var opts []tea.ProgramOption
	opts = append(opts, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if os.Getenv("PKM_DEBUG") != "" {
		if f, err := tea.LogToFile("pkm-debug.log", "pkm"); err == nil {
			defer f.Close()
		}
	}

	p := tea.NewProgram(m, opts...)
	tui.StartWatcher(v, idx, p)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "pkm: %v\n", err)
		os.Exit(1)
	}
}
