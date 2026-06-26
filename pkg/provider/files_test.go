package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	rusqslack "github.com/rusq/slack"
	slackdumpauth "github.com/rusq/slackdump/v3/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAuthProvider struct {
	token string
}

func (p testAuthProvider) SlackToken() string {
	return p.token
}

func (p testAuthProvider) Cookies() []*http.Cookie {
	return nil
}

func (p testAuthProvider) Validate() error {
	return nil
}

func (p testAuthProvider) Test(context.Context) (*rusqslack.AuthTestResponse, error) {
	return &rusqslack.AuthTestResponse{}, nil
}

func (p testAuthProvider) HTTPClient() (*http.Client, error) {
	return http.DefaultClient, nil
}

var _ slackdumpauth.Provider = testAuthProvider{}

func TestGetFileInfoRawContextPreservesRawFileMetadata(t *testing.T) {
	var gotAuth string
	var gotFile string
	var gotIncludeTranscription string
	var gotReason string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/files.info", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, r.ParseForm())
		gotFile = r.Form.Get("file")
		gotIncludeTranscription = r.Form.Get("include_transcription")
		gotReason = r.Form.Get("reason")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"ok": true,
			"file": {
				"id": "F0BC19QRWQ7",
				"filetype": "huddle_transcript",
				"mimetype": "application/vnd.slack-huddle-transcript",
				"huddle_transcription": {
					"lines": [{"start_time_ms": 164000, "user_id": "U123", "contents": "hello"}]
				}
			}
		}`))
	}))
	defer server.Close()

	client := &MCPSlackClient{
		httpClient:   server.Client(),
		authProvider: testAuthProvider{token: "xoxp-test"},
		teamEndpoint: server.URL + "/",
	}

	resp, err := client.GetFileInfoRawContext(context.Background(), "F0BC19QRWQ7", WithHuddleTranscription())
	require.NoError(t, err)
	require.True(t, resp.Ok)
	assert.Equal(t, "Bearer xoxp-test", gotAuth)
	assert.Equal(t, "F0BC19QRWQ7", gotFile)
	assert.Equal(t, "true", gotIncludeTranscription)
	assert.Equal(t, "slack-ai-fetch-huddle-transcript", gotReason)
	assert.Equal(t, "huddle_transcript", resp.File["filetype"])

	rawFile, ok := resp.Raw["file"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, rawFile, "huddle_transcription")
}

func TestGetFileInfoRawContextReturnsSlackErrorWithoutValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"ok": false,
			"error": "missing_scope",
			"needed": "files:read",
			"provided": "channels:history"
		}`))
	}))
	defer server.Close()

	client := &MCPSlackClient{
		httpClient:   server.Client(),
		authProvider: testAuthProvider{token: "xoxp-test"},
		teamEndpoint: server.URL + "/",
	}

	resp, err := client.GetFileInfoRawContext(context.Background(), "F0BC19QRWQ7")
	require.NoError(t, err)
	assert.False(t, resp.Ok)
	assert.Equal(t, "missing_scope", resp.Error)
	assert.Equal(t, "files:read", resp.Needed)
}
