package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type teamAPIResponse struct {
	Ok               bool   `json:"ok"`
	Error            string `json:"error,omitempty"`
	Needed           string `json:"needed,omitempty"`
	Provided         string `json:"provided,omitempty"`
	ResponseMetadata struct {
		NextCursor string `json:"next_cursor,omitempty"`
	} `json:"response_metadata,omitempty"`
}

type ListColumn struct {
	ID      string           `json:"id,omitempty"`
	Key     string           `json:"key,omitempty"`
	Name    string           `json:"name,omitempty"`
	Type    string           `json:"type,omitempty"`
	Options []map[string]any `json:"options,omitempty"`
}

type ListMetadata struct {
	Schema        []ListColumn `json:"schema,omitempty"`
	SubtaskSchema []ListColumn `json:"subtask_schema,omitempty"`
	TodoMode      any          `json:"todo_mode,omitempty"`
}

type ListFile struct {
	ID           string         `json:"id,omitempty"`
	Title        string         `json:"title,omitempty"`
	Name         string         `json:"name,omitempty"`
	Filetype     string         `json:"filetype,omitempty"`
	PrettyType   string         `json:"pretty_type,omitempty"`
	Mimetype     string         `json:"mimetype,omitempty"`
	User         string         `json:"user,omitempty"`
	Permalink    string         `json:"permalink,omitempty"`
	ListMetadata *ListMetadata  `json:"list_metadata,omitempty"`
	ListLimits   map[string]any `json:"list_limits,omitempty"`
}

type listInfoResponse struct {
	teamAPIResponse
	File *ListFile `json:"file,omitempty"`
}

type ListsItemsListResponse struct {
	teamAPIResponse
	Items []map[string]any `json:"items,omitempty"`
}

type ListsItemMutationResponse struct {
	teamAPIResponse
	Item  map[string]any   `json:"item,omitempty"`
	Items []map[string]any `json:"items,omitempty"`
}

func (c *MCPSlackClient) GetListInfoContext(ctx context.Context, listID string) (*ListFile, error) {
	resp := listInfoResponse{}
	if err := c.callTeamAPI(ctx, "files.info", map[string]any{
		"file": listID,
	}, &resp); err != nil {
		return nil, err
	}
	if resp.File == nil {
		return nil, fmt.Errorf("files.info returned no file for %q", listID)
	}
	if resp.File.Filetype != "list" {
		return nil, fmt.Errorf("%q is not a Slack List file", listID)
	}
	if resp.File.ListMetadata == nil {
		return nil, fmt.Errorf("Slack List metadata is unavailable for %q; ensure the token has files:read access", listID)
	}
	return resp.File, nil
}

func (c *MCPSlackClient) ListsItemsListContext(ctx context.Context, listID string, limit int, cursor string, archived bool) (*ListsItemsListResponse, error) {
	req := map[string]any{
		"list_id":  listID,
		"limit":    limit,
		"archived": archived,
	}
	if cursor != "" {
		req["cursor"] = cursor
	}

	resp := ListsItemsListResponse{}
	if err := c.callTeamAPI(ctx, "slackLists.items.list", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MCPSlackClient) ListsItemsCreateContext(ctx context.Context, listID string, initialFields []map[string]any, parentItemID string) (*ListsItemMutationResponse, error) {
	req := map[string]any{
		"list_id":        listID,
		"initial_fields": initialFields,
	}
	if parentItemID != "" {
		req["parent_item_id"] = parentItemID
	}

	resp := ListsItemMutationResponse{}
	if err := c.callTeamAPI(ctx, "slackLists.items.create", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MCPSlackClient) ListsItemsUpdateContext(ctx context.Context, listID string, cells []map[string]any) (*ListsItemMutationResponse, error) {
	resp := ListsItemMutationResponse{}
	if err := c.callTeamAPI(ctx, "slackLists.items.update", map[string]any{
		"list_id": listID,
		"cells":   cells,
	}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MCPSlackClient) callTeamAPI(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.teamEndpoint, "/")+"/api/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.authProvider.SlackToken())
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("%s: failed to decode response: %w", method, err)
	}

	base, ok := out.(interface{ responseBase() *teamAPIResponse })
	if ok {
		if err := validateTeamAPIResponse(method, base.responseBase()); err != nil {
			return err
		}
		return nil
	}

	return nil
}

func validateTeamAPIResponse(method string, resp *teamAPIResponse) error {
	if resp == nil || resp.Ok {
		return nil
	}

	msg := fmt.Sprintf("%s failed: %s", method, resp.Error)
	if resp.Needed != "" || resp.Provided != "" {
		msg = fmt.Sprintf("%s (needed=%s provided=%s)", msg, resp.Needed, resp.Provided)
	}
	return fmt.Errorf("%s", msg)
}

func (r *listInfoResponse) responseBase() *teamAPIResponse {
	return &r.teamAPIResponse
}

func (r *ListsItemsListResponse) responseBase() *teamAPIResponse {
	return &r.teamAPIResponse
}

func (r *ListsItemMutationResponse) responseBase() *teamAPIResponse {
	return &r.teamAPIResponse
}
