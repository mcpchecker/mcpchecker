package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyDir recursively copies the contents of src directory into dst directory.
// It preserves file permissions. dst is created if it does not exist.
func CopyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source %q: %w", src, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source %q is not a directory", src)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to create destination %q: %w", dst, err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read source directory %q: %w", src, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Resolve symlinks so we copy the target content as regular
		// files/directories rather than preserving the link itself.
		info, err := os.Stat(srcPath)
		if err != nil {
			return fmt.Errorf("failed to stat %q: %w", srcPath, err)
		}

		if info.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// MountSkills copies skill source directories into a target directory at the given mount path.
// It is a no-op if sourceDirs is empty.
func MountSkills(targetDir, mountPath string, sourceDirs []string) error {
	if len(sourceDirs) == 0 {
		return nil
	}

	if filepath.IsAbs(mountPath) {
		return fmt.Errorf("skill mount path must be relative, got %q", mountPath)
	}

	mountDir := filepath.Join(targetDir, filepath.Clean(mountPath))
	rel, err := filepath.Rel(targetDir, mountDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("skill mount path %q escapes target directory", mountPath)
	}

	if err := os.MkdirAll(mountDir, 0o755); err != nil {
		return fmt.Errorf("failed to create skill mount directory %q: %w", mountDir, err)
	}

	for _, srcDir := range sourceDirs {
		if err := CopyDir(srcDir, mountDir); err != nil {
			return fmt.Errorf("failed to copy skills from %q: %w", srcDir, err)
		}
	}

	return nil
}

func copyFile(src, dst string) (retErr error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %q: %w", src, err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file %q: %w", src, err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file %q: %w", dst, err)
	}
	defer func() {
		if closeErr := dstFile.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("failed to close destination file %q: %w", dst, closeErr)
		}
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy %q to %q: %w", src, dst, err)
	}

	return nil
}
