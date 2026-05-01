package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/genmcp/gen-mcp/pkg/template"
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

	env, err := expandEnv(spec.Env)
	if err != nil {
		return nil, err
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

	expandedConfig, err := expandConfig(spec.Config)
	if err != nil {
		return nil, err
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

// expandEnv resolves template references in the given env map and returns
// the result merged with the current OS environment as KEY=VALUE pairs.
func expandEnv(envMap map[string]string) ([]string, error) {
	env := os.Environ()
	for k, v := range envMap {
		result, err := expandTemplate(v)
		if err != nil {
			return nil, err
		}
		str, ok := result.(string)
		if !ok {
			return nil, fmt.Errorf("env template resolved to non-string type: %T", result)
		}
		env = append(env, fmt.Sprintf("%s=%s", k, str))
	}
	return env, nil
}

// expandConfig returns a deep copy of config with all string values resolved
// through template expansion, iterating nested maps and slices.
func expandConfig(config map[string]any) (map[string]any, error) {
	if config == nil {
		return nil, nil
	}

	expanded := make(map[string]any, len(config))
	for k, v := range config {
		expanded[k] = v
	}

	stack := []any{expanded}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch c := current.(type) {
		case map[string]any:
			for k, v := range c {
				switch t := v.(type) {
				case string:
					result, err := expandTemplate(t)
					if err != nil {
						return nil, err
					}
					c[k] = result
				case map[string]any:
					clone := make(map[string]any, len(t))
					for ck, cv := range t {
						clone[ck] = cv
					}
					c[k] = clone
					stack = append(stack, clone)
				case []any:
					clone := make([]any, len(t))
					copy(clone, t)
					c[k] = clone
					stack = append(stack, clone)
				}
			}
		case []any:
			for i, v := range c {
				switch t := v.(type) {
				case string:
					result, err := expandTemplate(t)
					if err != nil {
						return nil, err
					}
					c[i] = result
				case map[string]any:
					clone := make(map[string]any, len(t))
					for ck, cv := range t {
						clone[ck] = cv
					}
					c[i] = clone
					stack = append(stack, clone)
				case []any:
					clone := make([]any, len(t))
					copy(clone, t)
					c[i] = clone
					stack = append(stack, clone)
				}
			}
		}
	}

	return expanded, nil
}

// expandTemplate parses and resolves a template string, returning the expanded value.
func expandTemplate(s string) (any, error) {
	parsed, err := template.ParseTemplate(s, template.TemplateParserOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	builder, err := template.NewTemplateBuilder(parsed, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create template builder: %w", err)
	}
	return builder.GetResult()
}
