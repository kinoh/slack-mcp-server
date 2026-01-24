package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/server/auth"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

const (
	defaultSearchFilesLimit = 20
	maxSearchFilesLimit     = 100
)

type FileSearchResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Name       string `json:"name"`
	Filetype   string `json:"filetype"`
	PrettyType string `json:"prettyType"`
	Mimetype   string `json:"mimetype"`
	User       string `json:"user"`
	Channels   string `json:"channels"`
	Groups     string `json:"groups"`
	IMs        string `json:"ims"`
	Created    int64  `json:"created"`
	Timestamp  int64  `json:"timestamp"`
	Size       int    `json:"size"`
	IsPublic   bool   `json:"isPublic"`
	IsExternal bool   `json:"isExternal"`
	Editable   bool   `json:"editable"`
	URL        string `json:"url"`
	Permalink  string `json:"permalink"`
	Cursor     string `json:"cursor"`
}

type searchFilesParams struct {
	query string
	limit int
	page  int
}

type FilesHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewFilesHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *FilesHandler {
	return &FilesHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

func (fh *FilesHandler) SearchFilesAndCanvasesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fh.logger.Debug("SearchFilesAndCanvasesHandler called", zap.Any("params", request.Params.Arguments))

	if authenticated, err := auth.IsAuthenticated(ctx, fh.apiProvider.ServerTransport(), fh.logger); !authenticated {
		fh.logger.Error("Authentication failed for search_files_and_canvases", zap.Error(err))
		return mcp.NewToolResultError(err.Error()), nil
	}

	if ready, err := fh.apiProvider.IsReady(); !ready {
		fh.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultError(err.Error()), nil
	}

	params, err := fh.parseParamsToolSearchFiles(request)
	if err != nil {
		fh.logger.Error("Failed to parse search_files_and_canvases params", zap.Error(err))
		return nil, err
	}

	searchParams := slack.SearchParameters{
		Sort:          slack.DEFAULT_SEARCH_SORT,
		SortDirection: slack.DEFAULT_SEARCH_SORT_DIR,
		Highlight:     false,
		Count:         params.limit,
		Page:          params.page,
	}

	filesRes, err := fh.apiProvider.Slack().SearchFilesContext(ctx, params.query, searchParams)
	if err != nil {
		fh.logger.Error("Slack SearchFilesContext failed", zap.Error(err))
		return nil, err
	}

	results := make([]FileSearchResult, 0, len(filesRes.Matches))
	for _, file := range filesRes.Matches {
		results = append(results, FileSearchResult{
			ID:         file.ID,
			Title:      file.Title,
			Name:       file.Name,
			Filetype:   file.Filetype,
			PrettyType: file.PrettyType,
			Mimetype:   file.Mimetype,
			User:       file.User,
			Channels:   strings.Join(file.Channels, "|"),
			Groups:     strings.Join(file.Groups, "|"),
			IMs:        strings.Join(file.IMs, "|"),
			Created:    int64(file.Created),
			Timestamp:  int64(file.Timestamp),
			Size:       file.Size,
			IsPublic:   file.IsPublic,
			IsExternal: file.IsExternal,
			Editable:   file.Editable,
			URL:        file.URLPrivate,
			Permalink:  file.Permalink,
		})
	}

	if len(results) > 0 && filesRes.Pagination.Page < filesRes.Pagination.PageCount {
		nextCursor := fmt.Sprintf("page:%d", filesRes.Pagination.Page+1)
		results[len(results)-1].Cursor = base64.StdEncoding.EncodeToString([]byte(nextCursor))
	}

	csvBytes, err := gocsv.MarshalBytes(&results)
	if err != nil {
		fh.logger.Error("Failed to marshal files to CSV", zap.Error(err))
		return nil, err
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}

func (fh *FilesHandler) parseParamsToolSearchFiles(req mcp.CallToolRequest) (*searchFilesParams, error) {
	rawQuery := strings.TrimSpace(req.GetString("search_query", ""))
	freeText, filters := splitQuery(rawQuery)

	if chName := req.GetString("filter_in_channel", ""); chName != "" {
		f, err := fh.paramFormatChannel(chName)
		if err != nil {
			fh.logger.Error("Invalid channel filter", zap.String("filter", chName), zap.Error(err))
			return nil, err
		}
		addFilter(filters, "in", f)
	}
	if from := req.GetString("filter_users_from", ""); from != "" {
		f, err := fh.paramFormatUser(from)
		if err != nil {
			fh.logger.Error("Invalid from-user filter", zap.String("filter", from), zap.Error(err))
			return nil, err
		}
		addFilter(filters, "from", f)
	}

	dateMap, err := buildDateFilters(
		req.GetString("filter_date_before", ""),
		req.GetString("filter_date_after", ""),
		"",
		"",
	)
	if err != nil {
		fh.logger.Error("Invalid date filters", zap.Error(err))
		return nil, err
	}
	for key, val := range dateMap {
		addFilter(filters, key, val)
	}

	finalQuery := buildQuery(freeText, filters)
	if strings.TrimSpace(finalQuery) == "" {
		return nil, errors.New("search_query or filters must be provided")
	}

	limit := req.GetInt("limit", defaultSearchFilesLimit)
	if limit <= 0 {
		limit = defaultSearchFilesLimit
	}
	if limit > maxSearchFilesLimit {
		limit = maxSearchFilesLimit
	}

	cursor := req.GetString("cursor", "")
	page := 1
	if cursor != "" {
		decodedCursor, err := base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			fh.logger.Error("Invalid cursor decoding", zap.String("cursor", cursor), zap.Error(err))
			return nil, fmt.Errorf("invalid cursor: %v", err)
		}
		parts := strings.Split(string(decodedCursor), ":")
		if len(parts) != 2 {
			fh.logger.Error("Invalid cursor format", zap.String("cursor", cursor))
			return nil, fmt.Errorf("invalid cursor: %v", cursor)
		}
		page, err = strconv.Atoi(parts[1])
		if err != nil || page < 1 {
			fh.logger.Error("Invalid cursor page", zap.String("cursor", cursor), zap.Error(err))
			return nil, fmt.Errorf("invalid cursor page: %v", err)
		}
	}

	fh.logger.Debug("Search files parameters built",
		zap.String("query", finalQuery),
		zap.Int("limit", limit),
		zap.Int("page", page),
	)

	return &searchFilesParams{
		query: finalQuery,
		limit: limit,
		page:  page,
	}, nil
}

func (fh *FilesHandler) paramFormatUser(raw string) (string, error) {
	users := fh.apiProvider.ProvideUsersMap()
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "U") {
		u, ok := users.Users[raw]
		if !ok {
			return "", fmt.Errorf("user %q not found", raw)
		}
		return fmt.Sprintf("<@%s>", u.ID), nil
	}
	if strings.HasPrefix(raw, "<@") {
		raw = raw[2:]
	}
	if strings.HasPrefix(raw, "@") {
		raw = raw[1:]
	}
	uid, ok := users.UsersInv[raw]
	if !ok {
		return "", fmt.Errorf("user %q not found", raw)
	}
	return fmt.Sprintf("<@%s>", uid), nil
}

func (fh *FilesHandler) paramFormatChannel(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	cms := fh.apiProvider.ProvideChannelsMaps()
	if strings.HasPrefix(raw, "#") {
		if id, ok := cms.ChannelsInv[raw]; ok {
			return cms.Channels[id].Name, nil
		}
		return "", fmt.Errorf("channel %q not found", raw)
	}
	if strings.HasPrefix(raw, "C") || strings.HasPrefix(raw, "G") {
		if chn, ok := cms.Channels[raw]; ok {
			return chn.Name, nil
		}
		return "", fmt.Errorf("channel %q not found", raw)
	}
	return "", fmt.Errorf("invalid channel format: %q", raw)
}
