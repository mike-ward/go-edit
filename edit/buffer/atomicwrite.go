package buffer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// atomicWrite writes data to path atomically: write to a temp file in
// the same directory, sync, then rename. Falls back to direct write
// if rename fails (e.g. cross-device). Resolves symlinks so the
// target file is replaced, not the link.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	// Resolve symlinks to write to the real target.
	initial, err := symlinkSnapshot(path)
	if err != nil {
		return err
	}
	resolved := initial.resolved
	dir := filepath.Dir(resolved)

	tmp, err := os.CreateTemp(dir, ".go-edit-*.tmp")
	if err != nil {
		return fmt.Errorf("buffer: create temp: %w", err)
	}
	tmpName := tmp.Name()

	// Clean up temp file on any failure path.
	success := false
	defer func() {
		if !success {
			os.Remove(tmpName) //nolint:errcheck
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck
		return fmt.Errorf("buffer: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close() //nolint:errcheck
		return fmt.Errorf("buffer: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("buffer: close temp: %w", err)
	}

	if mode != 0 {
		// Best-effort; ignore EPERM on systems where the caller
		// doesn't own the file.
		_ = os.Chmod(tmpName, mode)
	}

	current, err := symlinkSnapshot(path)
	if err != nil {
		return fmt.Errorf("buffer: recheck symlink: %w", err)
	}
	if initial != current {
		return errors.New("buffer: save target changed during write")
	}

	if err := os.Rename(tmpName, resolved); err != nil {
		if initial.isSymlink {
			return fmt.Errorf("buffer: replace symlink target: %w", err)
		}
		// Fallback: direct write (non-atomic).
		return directWrite(resolved, data, mode)
	}
	success = true
	return nil
}

type symlinkState struct {
	resolved  string
	isSymlink bool
}

func symlinkSnapshot(path string) (symlinkState, error) {
	resolved, err := resolveSymlink(path)
	if err != nil {
		return symlinkState{}, err
	}

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return symlinkState{resolved: resolved}, nil
		}
		return symlinkState{}, err
	}
	return symlinkState{
		resolved:  resolved,
		isSymlink: info.Mode()&os.ModeSymlink != 0,
	}, nil
}

// resolveSymlink returns the target of path if it is a symlink,
// otherwise returns path unchanged.
func resolveSymlink(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil // new file
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// Try full resolution first (handles chains).
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			// Broken symlink — read the immediate target and
			// resolve relative to the link's directory.
			raw, err2 := os.Readlink(path)
			if err2 != nil {
				return "", fmt.Errorf("buffer: resolve symlink: %w", err)
			}
			if !filepath.IsAbs(raw) {
				raw = filepath.Join(filepath.Dir(path), raw)
			}
			return raw, nil
		}
		return target, nil
	}
	return path, nil
}

// directWrite is the non-atomic fallback.
func directWrite(path string, data []byte, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	return os.WriteFile(path, data, mode)
}
