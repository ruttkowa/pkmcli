// Package backup creates bounded, git-native vault backups.
package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const remoteName = "pkm-backup"

// Config describes one backup destination. Mode is "remote" or "path".
type Config struct {
	Mode        string
	Destination string
	Timeout     time.Duration
}

// Run snapshots all current vault changes into Git, then either pushes the
// current branch or atomically writes a portable bundle to a local path.
func Run(parent context.Context, root string, cfg Config) error {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()

	destination := strings.TrimSpace(cfg.Destination)
	if destination == "" {
		return errors.New("backup destination is not configured")
	}
	if err := ensureRepository(ctx, root); err != nil {
		return err
	}
	if err := snapshot(ctx, root); err != nil {
		return err
	}

	switch cfg.Mode {
	case "remote":
		return push(ctx, root, destination)
	case "path":
		return bundle(ctx, root, destination)
	default:
		return errors.New("backup mode is off")
	}
}

func ensureRepository(ctx context.Context, root string) error {
	if err := git(ctx, root, "rev-parse", "--git-dir"); err == nil {
		return nil
	}
	return git(ctx, root, "init")
}

func snapshot(ctx context.Context, root string) error {
	if err := git(ctx, root, "add", "-A"); err != nil {
		return fmt.Errorf("stage vault: %w", err)
	}
	cmd := command(ctx, root, "diff", "--cached", "--quiet")
	if err := cmd.Run(); err == nil {
		return nil
	} else if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 1 {
		return fmt.Errorf("inspect staged changes: %w", err)
	}
	return git(ctx, root,
		"-c", "user.name=pkm backup",
		"-c", "user.email=backup@localhost",
		"commit", "-m", "pkm backup "+time.Now().Format(time.RFC3339))
}

func push(ctx context.Context, root, destination string) error {
	if err := git(ctx, root, "remote", "get-url", remoteName); err == nil {
		if err := git(ctx, root, "remote", "set-url", remoteName, destination); err != nil {
			return fmt.Errorf("update backup remote: %w", err)
		}
	} else if err := git(ctx, root, "remote", "add", remoteName, destination); err != nil {
		return fmt.Errorf("add backup remote: %w", err)
	}
	if err := git(ctx, root, "push", remoteName, "HEAD"); err != nil {
		return fmt.Errorf("push backup: %w", err)
	}
	return nil
}

func bundle(ctx context.Context, root, destination string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absDestination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if absDestination == absRoot || strings.HasPrefix(absDestination, absRoot+string(os.PathSeparator)) {
		return errors.New("backup path must be outside the vault")
	}
	if info, err := os.Stat(absDestination); err != nil || !info.IsDir() {
		return errors.New("backup path is unavailable")
	}
	final := filepath.Join(absDestination, filepath.Base(absRoot)+".bundle")
	temp := final + ".tmp"
	if err := git(ctx, root, "bundle", "create", temp, "--all"); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("create bundle: %w", err)
	}
	if err := os.Rename(temp, final); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("publish bundle: %w", err)
	}
	return nil
}

func git(ctx context.Context, root string, args ...string) error {
	cmd := command(ctx, root, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%s", message)
		}
	}
	return err
}

func command(ctx context.Context, root string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	// Backups run behind the TUI. Never let Git open an invisible credential
	// prompt: SSH agents and configured credential helpers remain supported.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd
}
