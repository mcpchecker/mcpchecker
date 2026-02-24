package steps

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestRandomResolver_ID(t *testing.T) {
	r := NewRandomResolver()

	val, err := r.Resolve("id")
	if err != nil {
		t.Fatalf("Resolve(id) error: %v", err)
	}

	if len(val) != 8 {
		t.Errorf("expected 8-char id, got %q (len=%d)", val, len(val))
	}

	for _, c := range val {
		if !strings.ContainsRune(alphanumeric, c) {
			t.Errorf("id contains invalid character %q", string(c))
		}
	}
}

func TestRandomResolver_Port(t *testing.T) {
	r := NewRandomResolver()

	val, err := r.Resolve("port")
	if err != nil {
		t.Fatalf("Resolve(port) error: %v", err)
	}

	port, err := strconv.Atoi(val)
	if err != nil {
		t.Fatalf("port %q is not a number: %v", val, err)
	}

	if port < 1 || port > 65535 {
		t.Errorf("port %d is out of valid range", port)
	}

	// Verify the port was actually available (try to listen on it)
	l, err := net.Listen("tcp", "127.0.0.1:"+val)
	if err != nil {
		t.Errorf("port %s is not available: %v", val, err)
	} else {
		l.Close()
	}
}

func TestRandomResolver_Memoization(t *testing.T) {
	r := NewRandomResolver()

	id1, err := r.Resolve("id")
	if err != nil {
		t.Fatalf("first Resolve(id) error: %v", err)
	}

	id2, err := r.Resolve("id")
	if err != nil {
		t.Fatalf("second Resolve(id) error: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected memoized value, got %q and %q", id1, id2)
	}

	port1, err := r.Resolve("port")
	if err != nil {
		t.Fatalf("first Resolve(port) error: %v", err)
	}

	port2, err := r.Resolve("port")
	if err != nil {
		t.Fatalf("second Resolve(port) error: %v", err)
	}

	if port1 != port2 {
		t.Errorf("expected memoized port, got %q and %q", port1, port2)
	}
}

func TestRandomResolver_DifferentInstances(t *testing.T) {
	r1 := NewRandomResolver()
	r2 := NewRandomResolver()

	id1, _ := r1.Resolve("id")
	id2, _ := r2.Resolve("id")

	if id1 == id2 {
		t.Errorf("different resolvers produced same id %q", id1)
	}
}

func TestRandomResolver_UnknownField(t *testing.T) {
	r := NewRandomResolver()

	_, err := r.Resolve("unknown")
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}
