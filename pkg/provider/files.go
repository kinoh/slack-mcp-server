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

func (c *MCPSlackClient) GetFileInfoRawContext(ctx context.Context, fileID string) (*FileInfoRawResponse, error) {
	resp := FileInfoRawResponse{}
	if err := c.callTeamAPIForm(ctx, "files.info", url.Values{
		"token": {c.authProvider.SlackToken()},
		"file":  {fileID},
		"count": {"0"},
		"page":  {"0"},
	}, &resp); err != nil {
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
