package buffer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	if err := atomicWrite(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q", string(got))
	}
}

func TestAtomicWriteOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("old"), 0o644) //nolint:errcheck

	if err := atomicWrite(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("got %q", string(got))
	}
}

func TestAtomicWritePreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mode.txt")

	if err := atomicWrite(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Check executable bit (ignore umask effects on other bits).
	if info.Mode()&0o100 == 0 {
		t.Errorf("mode = %o, expected executable", info.Mode())
	}
}

func TestAtomicWriteBrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "broken.txt")

	// Symlink to nonexistent target.
	target := filepath.Join(dir, "gone.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	// Should succeed — resolves link, writes to target path.
	if err := atomicWrite(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Errorf("target = %q", string(got))
	}
}

func TestAtomicWriteSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")

	os.WriteFile(target, []byte("old"), 0o644) //nolint:errcheck
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	if err := atomicWrite(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Target should have new content.
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Errorf("target = %q", string(got))
	}

	// Link should still be a symlink.
	info, _ := os.Lstat(link)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("link is no longer a symlink")
	}
}

func TestSymlinkSnapshotDetectsRetarget(t *testing.T) {
	dir := t.TempDir()
	targetA := filepath.Join(dir, "target-a.txt")
	targetB := filepath.Join(dir, "target-b.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(targetA, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetB, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetA, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	first, err := symlinkSnapshot(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetB, link); err != nil {
		t.Fatal(err)
	}
	second, err := symlinkSnapshot(link)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected symlink snapshot to change after retarget")
	}
}
