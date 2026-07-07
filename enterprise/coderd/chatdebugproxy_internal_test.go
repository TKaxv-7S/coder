package coderd

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatDebugProxyURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		address string
		reqURL  string
		want    string
	}{
		{
			name:    "bare host and port",
			address: "http://10.0.0.1:8080",
			reqURL:  "/api/experimental/chats/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/debug/snapshot",
			want:    "http://10.0.0.1:8080/api/experimental/chats/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/debug/snapshot",
		},
		{
			name:    "address with base path is preserved, not overwritten",
			address: "http://10.0.0.1:8080/coder",
			reqURL:  "/api/experimental/chats/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/debug/snapshot",
			want:    "http://10.0.0.1:8080/coder/api/experimental/chats/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/debug/snapshot",
		},
		{
			name:    "query string is forwarded",
			address: "http://10.0.0.1:8080",
			reqURL:  "/api/experimental/chats/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/debug/snapshot?foo=bar",
			want:    "http://10.0.0.1:8080/api/experimental/chats/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/debug/snapshot?foo=bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseURL, err := url.Parse(tt.address)
			require.NoError(t, err)
			reqURL, err := url.Parse(tt.reqURL)
			require.NoError(t, err)

			got := chatDebugProxyURL(baseURL, reqURL)
			require.Equal(t, tt.want, got.String())
		})
	}
}
