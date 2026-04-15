package eval

import "testing"

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "empty string",
			raw:  "",
			want: "",
		},
		{
			name: "clean URL",
			raw:  "http://localhost:8080/mcp",
			want: "http://localhost:8080/mcp",
		},
		{
			name: "strips query params",
			raw:  "http://localhost:8080/mcp?token=secret123&debug=true",
			want: "http://localhost:8080/mcp",
		},
		{
			name: "strips userinfo",
			raw:  "http://admin:password@localhost:8080/mcp",
			want: "http://localhost:8080/mcp",
		},
		{
			name: "strips both query and userinfo",
			raw:  "https://user:pass@example.com/api?key=abc",
			want: "https://example.com/api",
		},
		{
			name: "strips fragment",
			raw:  "http://localhost:8080/mcp#section",
			want: "http://localhost:8080/mcp",
		},
		{
			name: "preserves path",
			raw:  "http://localhost:8080/v1/mcp/endpoint",
			want: "http://localhost:8080/v1/mcp/endpoint",
		},
		{
			name: "preserves port",
			raw:  "http://localhost:9090/mcp",
			want: "http://localhost:9090/mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeURL(tt.raw)
			if got != tt.want {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
