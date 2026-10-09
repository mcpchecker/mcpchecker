package mcpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandEnv(t *testing.T) {
	t.Setenv("FOO", "bar")
	assert.Equal(t, "bar", expandEnv("${FOO}"))
	assert.Equal(t, "fallback", expandEnv("${MISSING:-fallback}"))
	assert.Equal(t, "plain", expandEnv("plain"))
	assert.Equal(t, "fallback", expandEnv("${EMPTY:-fallback}"))
}

func TestResolveExpandsOnce(t *testing.T) {
	t.Setenv("SECRET", "has ${NOT_A_VAR} inside")
	cfg := &ServerConfig{
		Type: TransportTypeHttp,
		URL:  "http://example.com/mcp",
		Auth: &AuthConfig{
			Type:         "client_credentials",
			TokenURL:     "http://example.com/token",
			ClientID:     "id",
			ClientSecret: "${SECRET}",
		},
	}
	require.NoError(t, cfg.Resolve())
	require.NoError(t, cfg.Resolve())
	assert.Equal(t, "has ${NOT_A_VAR} inside", cfg.Auth.ClientSecret)
}

func TestParseConfigResolvesAuthEnv(t *testing.T) {
	t.Setenv("MCPCHECKER_TEST_URL", "http://example.com/mcp")
	t.Setenv("MCPCHECKER_TEST_ID", "my-id")

	raw := []byte(`{
  "mcpServers": {
    "demo": {
      "type": "http",
      "url": "${MCPCHECKER_TEST_URL}",
      "auth": {
        "type": "client_credentials",
        "tokenUrl": "http://token.example/oauth/token",
        "clientId": "${MCPCHECKER_TEST_ID}",
        "clientSecret": "secret"
      }
    }
  }
}`)
	cfg, err := ParseConfig(raw)
	require.NoError(t, err)
	srv := cfg.MCPServers["demo"]
	assert.Equal(t, "http://example.com/mcp", srv.URL)
	assert.Equal(t, "my-id", srv.Auth.ClientID)
}

func TestParseConfigRejectsUnknownAuth(t *testing.T) {
	raw := []byte(`{"mcpServers":{"x":{"type":"http","url":"http://localhost/mcp","auth":{"type":"password"}}}}`)
	_, err := ParseConfig(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported auth type")
}

func TestNewHTTPClient_StaticHeaderUnchanged(t *testing.T) {
	var saw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewHTTPClient(&ServerConfig{
		Type:    TransportTypeHttp,
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer static"},
	})
	require.NoError(t, err)
	resp, err := client.Get(srv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "Bearer static", saw)
}

func TestNewHTTPClient_ClientCredentials(t *testing.T) {
	tokenSrv, mcpSrv := newTokenAndMCP(t, []string{"tok-1"}, map[string]int{"tok-1": http.StatusOK})
	defer tokenSrv.Close()
	defer mcpSrv.Close()

	client := newAuthClient(t, tokenSrv.URL, mcpSrv.URL)
	resp, err := client.Get(mcpSrv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNewHTTPClient_RefreshesBeforeExpiry(t *testing.T) {
	var n atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
		id := n.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok-`+strconv.Itoa(int(id))+`","token_type":"Bearer","expires_in":1}`)
	}))
	defer tokenSrv.Close()

	var last atomic.Value
	mcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer mcpSrv.Close()

	client := newAuthClient(t, tokenSrv.URL, mcpSrv.URL)
	for range 2 {
		resp, err := client.Get(mcpSrv.URL)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
	assert.Equal(t, int32(2), n.Load())
	assert.Equal(t, "Bearer tok-2", last.Load())
}

func TestNewHTTPClient_RetriesOnceOn401(t *testing.T) {
	tokenSrv, mcpSrv := newTokenAndMCP(t, []string{"tok-1", "tok-2"}, map[string]int{
		"tok-1": http.StatusUnauthorized,
		"tok-2": http.StatusOK,
	})
	defer tokenSrv.Close()
	defer mcpSrv.Close()

	client := newAuthClient(t, tokenSrv.URL, mcpSrv.URL)
	resp, err := client.Get(mcpSrv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNewHTTPClient_Resource(t *testing.T) {
	var got atomic.Value
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got.Store(r.FormValue("resource"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	mcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mcpSrv.Close()

	client := newAuthClient(t, tokenSrv.URL, mcpSrv.URL)
	resp, err := client.Get(mcpSrv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, mcpSrv.URL, got.Load())

	client, err = NewHTTPClient(&ServerConfig{
		Type: TransportTypeHttp,
		URL:  mcpSrv.URL,
		Auth: &AuthConfig{
			Type:         "client_credentials",
			TokenURL:     tokenSrv.URL,
			ClientID:     "id",
			ClientSecret: "secret",
			Resource:     "https://example.com/mcp",
		},
	})
	require.NoError(t, err)
	resp, err = client.Get(mcpSrv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "https://example.com/mcp", got.Load())
}

func TestNewHTTPClient_MissingCredentials(t *testing.T) {
	_, err := NewHTTPClient(&ServerConfig{
		Type: TransportTypeHttp,
		URL:  "http://example.com/mcp",
		Auth: &AuthConfig{Type: "client_credentials", TokenURL: "http://example.com/token"},
	})
	require.Error(t, err)
}

func newAuthClient(t *testing.T, tokenURL, mcpURL string) *http.Client {
	t.Helper()
	client, err := NewHTTPClient(&ServerConfig{
		Type: TransportTypeHttp,
		URL:  mcpURL,
		Auth: &AuthConfig{
			Type:         "client_credentials",
			TokenURL:     tokenURL,
			ClientID:     "id",
			ClientSecret: "secret",
		},
	})
	require.NoError(t, err)
	return client
}

func newTokenAndMCP(t *testing.T, tokens []string, statusForToken map[string]int) (*httptest.Server, *httptest.Server) {
	t.Helper()
	var n atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
		i := int(n.Add(1)) - 1
		if i >= len(tokens) {
			i = len(tokens) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+tokens[i]+`","token_type":"Bearer","expires_in":3600}`)
	}))
	mcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("Authorization")
		code := http.StatusUnauthorized
		for name, status := range statusForToken {
			if tok == "Bearer "+name {
				code = status
			}
		}
		w.WriteHeader(code)
	}))
	return tokenSrv, mcpSrv
}
