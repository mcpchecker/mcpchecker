package mcpclient

import (
	"context"
	"fmt"
	"io"
	"net"
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
	if cfg.Auth != nil {
		if err := cfg.Resolve(); err != nil {
			return nil, err
		}
	}
	base := http.DefaultTransport
	var extra http.Header
	if len(cfg.Headers) > 0 {
		extra = make(http.Header, len(cfg.Headers))
		for k, v := range cfg.Headers {
			extra.Set(k, v)
		}
	}

	// Reject redirects that leave the configured MCP server's origin. The
	// transports below re-attach credentials (a bearer token or static
	// headers such as Authorization/API keys) to every request they see,
	// which would otherwise also apply to a redirected request. Without this
	// check, a malicious or compromised MCP server could use a redirect to
	// exfiltrate those credentials to an unrelated host, including via an
	// https -> http downgrade.
	client := &http.Client{CheckRedirect: disallowCrossOriginRedirects}

	if cfg.Auth != nil {
		if cfg.Auth.Type != "client_credentials" {
			return nil, fmt.Errorf("unsupported auth type %q", cfg.Auth.Type)
		}
		tokenURL := cfg.Auth.TokenURL
		clientID := cfg.Auth.ClientID
		clientSecret := cfg.Auth.ClientSecret
		if tokenURL == "" || clientID == "" || clientSecret == "" {
			return nil, fmt.Errorf("client_credentials auth requires tokenUrl, clientId, and clientSecret")
		}
		if err := requireSecureURL("tokenUrl", tokenURL); err != nil {
			return nil, err
		}
		if err := requireSecureURL("url", cfg.URL); err != nil {
			return nil, err
		}
		var scopes []string
		if cfg.Auth.Scope != "" {
			scopes = strings.Fields(cfg.Auth.Scope)
		}
		resource := cfg.Auth.Resource
		if resource == "" {
			resource = cfg.URL
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
			// The token endpoint is a separate trust boundary from the MCP
			// server itself. Without this, golang.org/x/oauth2 would fall
			// back to http.DefaultClient for the token request, which has no
			// redirect protection: a malicious/compromised token endpoint
			// could 307/308-redirect the request (Go preserves the POST body
			// on those codes) and receive clientId/clientSecret at an
			// unrelated host if the exchange falls back to sending them in
			// the request body.
			httpClient: &http.Client{CheckRedirect: disallowCrossOriginRedirects},
		}
		delete(extra, "Authorization")
		inner := base
		if len(extra) > 0 {
			inner = NewHeaderRoundTripper(extra, base)
		}
		client.Transport = &bearerTransport{source: src, base: inner}
		return client, nil
	}

	if len(extra) > 0 {
		client.Transport = NewHeaderRoundTripper(extra, base)
		return client, nil
	}
	client.Transport = base
	return client, nil
}

// disallowCrossOriginRedirects implements http.Client.CheckRedirect. It
// rejects any redirect whose scheme, host, or port differs from the first
// request in the chain (i.e. the configured MCP server URL), including an
// https -> http downgrade on the same host.
func disallowCrossOriginRedirects(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	orig := via[0].URL
	if req.URL.Scheme != orig.Scheme || req.URL.Host != orig.Host {
		return fmt.Errorf("refusing to follow cross-origin redirect from %s to %s", orig, req.URL)
	}
	return nil
}

// requireSecureURL rejects plaintext http:// URLs so OAuth credentials (the
// client secret sent to tokenUrl, or the bearer token sent to the MCP
// server) are never sent in cleartext. http is still allowed for loopback
// hosts so local development and tests (e.g. httptest servers) keep working.
func requireSecureURL(field, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", field, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("%s must use https when auth is configured (http is allowed only for loopback hosts)", field)
}

// refreshingTokenSource caches a client-credentials token until it is near
// expiry. reset drops the cache so the next request fetches a new token.
type refreshingTokenSource struct {
	conf *clientcredentials.Config
	// httpClient is used for the token endpoint request itself; see the
	// comment where this is constructed for why it needs its own
	// redirect protection.
	httpClient *http.Client

	mu  sync.Mutex
	tok *oauth2.Token
}

func (s *refreshingTokenSource) Token(ctx context.Context) (*oauth2.Token, error) {
	s.mu.Lock()
	tok := s.tok
	s.mu.Unlock()
	if tok.Valid() {
		return tok, nil
	}
	// A new TokenSource starts empty, so this always hits the token endpoint
	// when our cache is empty or the cached token is no longer valid.
	// ctx is the MCP request context, so cancelling that request cancels the fetch.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.httpClient)
	fresh, err := s.conf.TokenSource(ctx).Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.tok = fresh
	s.mu.Unlock()
	return fresh, nil
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
	tok, err := t.source.Token(req.Context())
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
		if req.Body != nil {
			req.Body.Close()
			req.Body = nil
		}
	}
	tok.SetAuthHeader(cloned)
	return t.base.RoundTrip(cloned)
}
