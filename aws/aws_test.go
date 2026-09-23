package aws

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_HTTPClient(t *testing.T) {
	tests := []struct {
		name       string
		httpClient *http.Client
		wantErr    error
	}{
		{
			name:       "fail when self-signed endpoint is used without insecure http client",
			httpClient: nil,
			wantErr:    errAny,
		},
		{
			name: "succeed when insecure http client is provided",
			httpClient: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`))
			}))
			defer server.Close()

			endpoint := strings.TrimPrefix(server.URL, "https://")

			client, err := NewClient(
				t.Context(), "https://"+endpoint, "us-east-1", "x", "y", true, "supported", tt.httpClient,
			)
			require.NoError(t, err)

			err = client.S3.List(t.Context(), S3ListOptions{Path: ""}, func(string) error { return nil })
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "x509")

				return
			}

			assert.NoError(t, err)
		})
	}
}
