package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransportFlagCanonical(t *testing.T) {
	tests := []struct {
		name      string
		values    []string
		expected  string
		wantError string
	}{
		{
			name:     "defaults to stdio",
			expected: "stdio",
		},
		{
			name:     "accepts sse",
			values:   []string{"sse"},
			expected: "sse",
		},
		{
			name:     "accepts http",
			values:   []string{"http"},
			expected: "http",
		},
		{
			name:     "accepts comma separated sse and http",
			values:   []string{"sse,http"},
			expected: "sse,http",
		},
		{
			name:     "accepts repeated sse and http flags",
			values:   []string{"sse", "http"},
			expected: "sse,http",
		},
		{
			name:     "canonicalizes http before sse",
			values:   []string{"http,sse"},
			expected: "sse,http",
		},
		{
			name:      "rejects stdio with http transport",
			values:    []string{"stdio,http"},
			wantError: "stdio cannot be combined",
		},
		{
			name:      "rejects stdio with sse transport",
			values:    []string{"stdio", "sse"},
			wantError: "stdio cannot be combined",
		},
		{
			name:      "rejects duplicate transport",
			values:    []string{"sse,sse"},
			wantError: "duplicate transport",
		},
		{
			name:      "rejects unknown transport",
			values:    []string{"websocket"},
			wantError: "unsupported transport",
		},
		{
			name:      "rejects empty transport",
			values:    []string{""},
			wantError: "empty transport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transports := newTransportFlag()
			for _, value := range tt.values {
				err := transports.Set(value)
				if tt.wantError != "" && err != nil {
					assert.Contains(t, err.Error(), tt.wantError)
					return
				}
				require.NoError(t, err)
			}

			actual, err := transports.canonical()
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestProviderTransport(t *testing.T) {
	assert.Equal(t, "stdio", providerTransport("stdio"))
	assert.Equal(t, "sse", providerTransport("sse"))
	assert.Equal(t, "http", providerTransport("http"))
	assert.Equal(t, "http", providerTransport("sse,http"))
	assert.Equal(t, "http", providerTransport("http,sse"))
}
