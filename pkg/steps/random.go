package steps

import (
	"crypto/rand"
	"fmt"
	"net"
	"sync"
)

const alphanumeric = "abcdefghijklmnopqrstuvwxyz0123456789"

// RandomResolver resolves {random.id} and {random.port} template variables.
// Values are memoized so the same variable returns the same value within a
// single task execution.
type RandomResolver struct {
	mu     sync.Mutex
	values map[string]string
}

// NewRandomResolver creates a new RandomResolver with empty memoization cache.
func NewRandomResolver() *RandomResolver {
	return &RandomResolver{
		values: make(map[string]string),
	}
}

// Resolve returns the value for a random template variable.
// Supported fields: "id" (8-char alphanumeric) and "port" (available TCP port).
func (r *RandomResolver) Resolve(fieldName string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if val, ok := r.values[fieldName]; ok {
		return val, nil
	}

	var val string
	var err error

	switch fieldName {
	case "id":
		val, err = generateRandomID(8)
	case "port":
		val, err = findAvailablePort()
	default:
		return "", fmt.Errorf("unknown random field %q: supported fields are \"id\" and \"port\"", fieldName)
	}

	if err != nil {
		return "", err
	}

	r.values[fieldName] = val
	return val, nil
}

// generateRandomID returns a random lowercase alphanumeric string of the given length.
func generateRandomID(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random id: %w", err)
	}
	for i := range b {
		b[i] = alphanumeric[int(b[i])%len(alphanumeric)]
	}
	return string(b), nil
}

// findAvailablePort returns an available TCP port as a string.
func findAvailablePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to find available port: %w", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	return fmt.Sprintf("%d", port), nil
}
