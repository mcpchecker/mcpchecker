package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/mcpchecker/mcpchecker/pkg/extension"
	"github.com/mcpchecker/mcpchecker/pkg/extension/protocol"
	"github.com/mcpchecker/mcpchecker/pkg/extension/resolver"
)

type ExtensionManager interface {
	// Register adds an extension specification
	Register(alias string, spec *extension.ExtensionSpec) error
	// Get returns a running client by alias, starting it if needed
	Get(ctx context.Context, pkg string) (Client, error)
	// Has returns whether an extension is registered
	Has(alias string) bool
	// ShutdownAll stops all running extensions
	ShutdownAll(ctx context.Context) error
}

type extensionManager struct {
	mu       sync.Mutex
	clients  map[string]Client
	specs    map[string]*extension.ExtensionSpec
	resolver resolver.Resolver
	opts     ExtensionOptions
}

type ExtensionOptions struct {
	LogHandler func(pkg, level, message string, data map[string]any)
}

func NewManager(res resolver.Resolver, opts ExtensionOptions) ExtensionManager {
	return &extensionManager{
		clients:  make(map[string]Client),
		specs:    make(map[string]*extension.ExtensionSpec),
		resolver: res,
		opts:     opts,
	}
}

func (m *extensionManager) Register(alias string, spec *extension.ExtensionSpec) error {
	if spec == nil {
		return fmt.Errorf("extension spec is required")
	}
	if spec.Package == "" {
		return fmt.Errorf("extension spec: package field is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.specs[alias]; exists {
		return fmt.Errorf("extension alias %q already registered", alias)
	}

	m.specs[alias] = spec
	return nil
}

func (m *extensionManager) Get(ctx context.Context, alias string) (Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.clients[alias]; ok {
		return c, nil
	}

	spec, ok := m.specs[alias]
	if !ok {
		return nil, fmt.Errorf("no extension registered for alias %q", alias)
	}

	binaryPath, err := m.resolver.Resolve(ctx, spec.Package)
	if err != nil {
		return nil, err
	}

	env := os.Environ()
	for k, v := range spec.Env {
		v = os.ExpandEnv(v)
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	c := New(Options{
		BinaryPath: binaryPath,
		LogHandler: func(level, message string, data map[string]any) {
			if m.opts.LogHandler != nil {
				m.opts.LogHandler(spec.Package, level, message, data)
			}
		},
		Env: env,
	})

	expandedConfig := spec.Config
	if spec.Config != nil {
		expandedConfig = make(map[string]any)
		for k, v := range spec.Config {
			switch t := v.(type) {
			case string:
				expandedConfig[k] = os.ExpandEnv(t)
			}
		}
	}

	if err := c.Start(ctx, &protocol.InitializeParams{
		Config: expandedConfig,
	}); err != nil {
		return nil, err
	}

	m.clients[alias] = c
	return c, nil
}

func (m *extensionManager) Has(alias string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.specs[alias]

	return ok
}

func (m *extensionManager) ShutdownAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for pkg, c := range m.clients {
		if err := c.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pkg, err))
		}
		delete(m.clients, pkg)
	}

	return errors.Join(errs...)
}

type managerKey struct{}

func ManagerToContext(ctx context.Context, manager ExtensionManager) context.Context {
	return context.WithValue(ctx, managerKey{}, manager)
}

func ManagerFromContext(ctx context.Context) (ExtensionManager, bool) {
	manager, ok := ctx.Value(managerKey{}).(ExtensionManager)
	return manager, ok
}
