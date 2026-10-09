package mcpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// NewHTTPClient builds the HTTP client used to reach one MCP server.
// Static headers keep working. An auth block replaces a static Authorization
// header with a client-credentials token that is refreshed before expiry and
// once after a 401.
func NewHTTPClient(cfg *ServerConfig) (*http.Client, error) {
	base := http.DefaultTransport
	var extra http.Header
	if len(cfg.Headers) > 0 {
		extra = make(http.Header, len(cfg.Headers))
		for k, v := range cfg.Headers {
			extra.Set(k, v)
		}
	}

	if cfg.Auth != nil {
		if cfg.Auth.Type != "client_credentials" {
			return nil, fmt.Errorf("unsupported auth type %q", cfg.Auth.Type)
		}
		tokenURL := expandEnv(cfg.Auth.TokenURL)
		clientID := expandEnv(cfg.Auth.ClientID)
		clientSecret := expandEnv(cfg.Auth.ClientSecret)
		if tokenURL == "" || clientID == "" || clientSecret == "" {
			return nil, fmt.Errorf("client_credentials auth requires tokenUrl, clientId, and clientSecret")
		}
		var scopes []string
		if scope := expandEnv(cfg.Auth.Scope); scope != "" {
			scopes = strings.Fields(scope)
		}
		resource := expandEnv(cfg.Auth.Resource)
		if resource == "" {
			resource = expandEnv(cfg.URL)
		}
		var params url.Values
		if resource != "" {
			params = url.Values{"resource": {resource}}
		}
		src := &refreshingTokenSource{
			conf: &clientcredentials.Config{
				ClientID:       clientID,
				ClientSecret:   clientSecret,
				TokenURL:       tokenURL,
				Scopes:         scopes,
				EndpointParams: params,
			},
		}
		delete(extra, "Authorization")
		var inner http.RoundTripper = base
		if len(extra) > 0 {
			inner = NewHeaderRoundTripper(extra, base)
		}
		return &http.Client{Transport: &bearerTransport{source: src, base: inner}}, nil
	}

	if len(extra) > 0 {
		return &http.Client{Transport: NewHeaderRoundTripper(extra, base)}, nil
	}
	return &http.Client{Transport: base}, nil
}

// refreshingTokenSource caches a client-credentials token until it is near
// expiry. reset drops the cache so the next request fetches a new token.
type refreshingTokenSource struct {
	conf *clientcredentials.Config

	mu  sync.Mutex
	tok *oauth2.Token
}

func (s *refreshingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tok.Valid() {
		return s.tok, nil
	}
	// A new TokenSource starts empty, so this always hits the token endpoint
	// when our cache is empty or the cached token is no longer valid.
	tok, err := s.conf.TokenSource(context.Background()).Token()
	if err != nil {
		return nil, err
	}
	s.tok = tok
	return tok, nil
}

func (s *refreshingTokenSource) reset() {
	s.mu.Lock()
	s.tok = nil
	s.mu.Unlock()
}

type bearerTransport struct {
	source *refreshingTokenSource
	base   http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.roundTrip(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	t.source.reset()
	return t.roundTrip(req)
}

func (t *bearerTransport) roundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.source.Token()
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		cloned.Body = body
	}
	tok.SetAuthHeader(cloned)
	return t.base.RoundTrip(cloned)
}
