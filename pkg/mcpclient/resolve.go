package mcpclient

import (
	"fmt"
	"sync"
)

// resolved marks configs that have already been expanded. A second pass would
// treat a literal ${...} inside an expanded secret as another reference.
var resolved sync.Map

// Resolve expands environment references in server URLs, headers, and auth fields.
func (c *MCPConfig) Resolve() error {
	if c == nil || c.MCPServers == nil {
		return nil
	}
	for name, server := range c.MCPServers {
		if err := server.Resolve(); err != nil {
			return fmt.Errorf("server %q: %w", name, err)
		}
	}
	return nil
}

func (s *ServerConfig) Resolve() error {
	if s == nil {
		return nil
	}
	if _, ok := resolved.Load(s); ok {
		return nil
	}
	s.URL = expandEnv(s.URL)
	if s.Headers != nil {
		for k, v := range s.Headers {
			s.Headers[k] = expandEnv(v)
		}
	}
	if s.Auth != nil {
		s.Auth.TokenURL = expandEnv(s.Auth.TokenURL)
		s.Auth.ClientID = expandEnv(s.Auth.ClientID)
		s.Auth.ClientSecret = expandEnv(s.Auth.ClientSecret)
		s.Auth.Scope = expandEnv(s.Auth.Scope)
		s.Auth.Resource = expandEnv(s.Auth.Resource)
		if s.Auth.Type != "" && s.Auth.Type != "client_credentials" {
			return fmt.Errorf("unsupported auth type %q", s.Auth.Type)
		}
	}
	resolved.Store(s, struct{}{})
	return nil
}
