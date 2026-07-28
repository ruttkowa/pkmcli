package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pkm/internal/index"
	"pkm/internal/tui"
	"pkm/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	vaultPath := "."
	if len(os.Args) > 1 {
		vaultPath = os.Args[1]
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
