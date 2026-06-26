package handler

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSlackFileID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "raw file id",
			input: "F0BC19QRWQ7",
			want:  "F0BC19QRWQ7",
		},
		{
			name:  "docs permalink",
			input: "https://amiyacorp.slack.com/docs/T04Q15YEGDS/F0BCADASTRR",
			want:  "F0BCADASTRR",
		},
		{
			name:  "app file url with query",
			input: "https://app.slack.com/client/T123/C123/files/F0BC19QRWQ7?origin_team=T123",
			want:  "F0BC19QRWQ7",
		},
		{
			name:    "non Slack URL is rejected",
			input:   "https://example.com/docs/T123/F0BCADASTRR",
			wantErr: true,
		},
		{
			name:    "non file id is rejected",
			input:   "C1234567890",
			wantErr: true,
		},
		{
			name:    "non https URL is rejected",
			input:   "http://amiyacorp.slack.com/docs/T04Q15YEGDS/F0BCADASTRR",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSlackFileID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLooksLikeHTML(t *testing.T) {
	assert.True(t, looksLikeHTML([]byte("<!doctype html><html><title>Slack</title>"), ""))
	assert.True(t, looksLikeHTML([]byte("plain"), "text/html; charset=utf-8"))
	assert.False(t, looksLikeHTML([]byte(`{"segments":[{"text":"hello"}]}`), "application/json"))
}

func TestParseTranscriptValue(t *testing.T) {
	value := map[string]any{
		"segments": []any{
			map[string]any{
				"offset":  "00:02:44",
				"user_id": "U123",
				"text":    "hello",
			},
		},
	}

	segments, err := parseTranscriptValue(value)
	require.NoError(t, err)
	require.Len(t, segments, 1)
	assert.Equal(t, "00:02:44", segments[0].Offset)
	assert.Equal(t, "U123", segments[0].UserID)
	assert.Equal(t, "hello", segments[0].Text)
}

func TestParseTranscriptBodyRejectsUnsupportedStructure(t *testing.T) {
	_, err := parseTranscriptBody([]byte(`{"not_transcript": true}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported transcript structure")
}

func TestExtractEmbeddedTranscriptFileID(t *testing.T) {
	file := map[string]any{
		"content": []any{
			map[string]any{
				"type":     "embedded-file",
				"filetype": "huddle_transcript",
				"id":       "F0BC19QRWQ7",
			},
		},
	}

	got, err := extractEmbeddedTranscriptFileID(file)
	require.NoError(t, err)
	assert.Equal(t, "F0BC19QRWQ7", got)
}

func TestClassifyDownloadError(t *testing.T) {
	assert.Equal(t, "missing_scope", classifyDownloadError(errors.New("missing_scope")))
	assert.Equal(t, "access_denied", classifyDownloadError(errors.New("download failed: 403")))
	assert.Equal(t, "file_not_found", classifyDownloadError(errors.New("file_not_found")))
	assert.Equal(t, "not_in_channel", classifyDownloadError(errors.New("not_in_channel")))
	assert.Equal(t, "not_visible", classifyDownloadError(errors.New("not_visible")))
	assert.Equal(t, "download_failed", classifyDownloadError(errors.New("connection reset")))
}
