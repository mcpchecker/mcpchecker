package mcpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient_RedirectLimit(t *testing.T) {
	for _, mode := range []string{"static headers", "oauth mcp", "oauth token"} {
		t.Run(mode, func(t *testing.T) {
			for _, redirects := range []int{9, 10} {
				t.Run(strconv.Itoa(redirects), func(t *testing.T) {
					redirectPath := "/mcp/"
					if mode == "oauth token" {
						redirectPath = "/token/"
					}
					var lastRedirectIndex atomic.Int32
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if strings.HasPrefix(r.URL.Path, redirectPath) {
							index, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, redirectPath))
							if !assert.NoError(t, err) {
								w.WriteHeader(http.StatusBadRequest)
								return
							}
							lastRedirectIndex.Store(int32(index))
							if index < redirects {
								http.Redirect(w, r, redirectPath+strconv.Itoa(index+1), http.StatusTemporaryRedirect)
								return
							}
						}
						if strings.HasPrefix(r.URL.Path, "/token/") {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, `{"access_token":"tok","token_type":"Bearer","expires_in":3600}`)
							return
						}
						if mode == "static headers" {
							assert.Equal(t, "Bearer static", r.Header.Get("Authorization"))
						} else {
							assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
						}
						w.WriteHeader(http.StatusOK)
					}))
					defer srv.Close()

					var client *http.Client
					if mode == "static headers" {
						var err error
						client, err = NewHTTPClient(&ServerConfig{
							Type:    TransportTypeHttp,
							URL:     srv.URL + "/mcp/0",
							Headers: map[string]string{"Authorization": "Bearer static"},
						})
						require.NoError(t, err)
					} else {
						client = newAuthClient(t, srv.URL+"/token/0", srv.URL+"/mcp/0")
					}
					resp, err := client.Get(srv.URL + "/mcp/0")
					if resp != nil {
						resp.Body.Close()
					}
					if redirects == 10 {
						require.ErrorContains(t, err, "stopped after 10 redirects")
					} else {
						require.NoError(t, err)
						assert.Equal(t, http.StatusOK, resp.StatusCode)
					}
					assert.Equal(t, int32(9), lastRedirectIndex.Load())
				})
			}
		})
	}
}
