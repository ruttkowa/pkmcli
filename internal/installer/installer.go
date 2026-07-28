package installer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
	Executable  string
	Destination string
	Home        string
	Shell       string
	Path        string
	In          io.Reader
	Out         io.Writer
}

// Install copies the running binary into Destination (default
// ~/.local/bin/pkm) and offers to persist that directory in the current
// shell's rc file when it is not already on PATH.
func Install(opts Options) error {
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Home == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		opts.Home = home
	}
	if opts.Executable == "" {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		opts.Executable = exe
	}
	if opts.Destination == "" {
		opts.Destination = filepath.Join(opts.Home, ".local", "bin", "pkm")
	}
	if err := os.MkdirAll(filepath.Dir(opts.Destination), 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}
	if err := copyExecutable(opts.Executable, opts.Destination); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "Installed pkm to %s\n", opts.Destination)

	dir := filepath.Dir(opts.Destination)
	if pathContains(opts.Path, dir) {
		return nil
	}
	rc := shellRC(opts.Home, opts.Shell)
	if rc == "" {
		fmt.Fprintf(opts.Out, "%s is not on PATH; add it in your shell configuration.\n", dir)
		return nil
	}
	fmt.Fprintf(opts.Out, "%s is not on PATH. Add it to %s? [Y/n] ", dir, rc)
	answer, _ := bufio.NewReader(opts.In).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "" && answer != "y" && answer != "yes" {
		fmt.Fprintf(opts.Out, "Skipped PATH update. Add: export PATH=\"%s:$PATH\"\n", dir)
		return nil
	}
	return appendPathLine(rc, dir)
}

func copyExecutable(source, destination string) error {
	sourceAbs, _ := filepath.Abs(source)
	destAbs, _ := filepath.Abs(destination)
	if sourceAbs == destAbs {
		return os.Chmod(destination, 0o755)
	}
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open current binary: %w", err)
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".pkm-install-*")
	if err != nil {
		return fmt.Errorf("create install file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}
	return nil
}

func pathContains(pathValue, dir string) bool {
	for _, entry := range filepath.SplitList(pathValue) {
		if clean, err := filepath.Abs(entry); err == nil && clean == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

func shellRC(home, shell string) string {
	switch filepath.Base(shell) {
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "bash":
		return filepath.Join(home, ".bashrc")
	default:
		return ""
	}
}

func appendPathLine(rc, dir string) error {
	line := fmt.Sprintf("export PATH=\"%s:$PATH\"", dir)
	if data, err := os.ReadFile(rc); err == nil {
		if strings.Contains(string(data), line) {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(rc, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if info, _ := f.Stat(); info != nil && info.Size() > 0 {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(line + "\n")
	return err
}
