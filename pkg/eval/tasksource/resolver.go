package tasksource

import "context"

// Resolver fetches external task sources and resolves them to local directories.
type Resolver interface {
	// Resolve fetches the source at the given repo and ref, returning the local
	// directory path containing the task files and the resolved commit SHA.
	Resolve(ctx context.Context, repo, ref string) (dir string, commit string, err error)

	// ContentHash computes a SHA-256 hash of the source content at the given
	// directory path, suitable for lockfile integrity verification.
	ContentHash(dir string) (string, error)
}
