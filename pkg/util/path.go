package util

import "path/filepath"

// ResolveRelativePath converts a relative path to absolute using basePath as the base directory.
// Absolute paths and empty strings are left unchanged.
func ResolveRelativePath(filePath *string, basePath string) error {
	if filePath == nil || *filePath == "" {
		return nil
	}

	if filepath.IsAbs(*filePath) {
		return nil
	}

	*filePath = filepath.Join(basePath, *filePath)

	return nil
}
