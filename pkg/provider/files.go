package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

type FileInfoRawResponse struct {
	Ok       bool           `json:"ok"`
	Error    string         `json:"error,omitempty"`
	Needed   string         `json:"needed,omitempty"`
	Provided string         `json:"provided,omitempty"`
	File     map[string]any `json:"file,omitempty"`
	Raw      map[string]any `json:"-"`
}

type FileInfoRawOption func(url.Values)

func WithHuddleTranscription() FileInfoRawOption {
	return func(values url.Values) {
		values.Set("include_transcription", "true")
		values.Set("reason", "slack-ai-fetch-huddle-transcript")
	}
}

func (c *MCPSlackClient) GetFileInfoRawContext(ctx context.Context, fileID string, options ...FileInfoRawOption) (*FileInfoRawResponse, error) {
	resp := FileInfoRawResponse{}
	values := url.Values{
		"token": {c.authProvider.SlackToken()},
		"file":  {fileID},
		"count": {"0"},
		"page":  {"0"},
	}
	for _, option := range options {
		option(values)
	}
	if err := c.callTeamAPIForm(ctx, "files.info", values, &resp); err != nil {
		return nil, err
	}
	if resp.Ok && resp.File == nil {
		return nil, fmt.Errorf("files.info returned no file for %q", fileID)
	}
	return &resp, nil
}

func (r *FileInfoRawResponse) UnmarshalJSON(data []byte) error {
	type alias FileInfoRawResponse
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = FileInfoRawResponse(a)
	r.Raw = raw
	return nil
}
