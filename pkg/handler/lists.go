package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

const (
	defaultListsItemsLimit = 50
	maxListsItemsLimit     = 100
)

type ListsHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewListsHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *ListsHandler {
	return &ListsHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

func (lh *ListsHandler) ListsItemsListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if ready, err := lh.apiProvider.IsReady(); !ready {
		lh.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultError(err.Error()), nil
	}

	listID, err := normalizeListID(request.GetString("list_id", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	limit := request.GetInt("limit", defaultListsItemsLimit)
	if limit <= 0 {
		limit = defaultListsItemsLimit
	}
	if limit > maxListsItemsLimit {
		limit = maxListsItemsLimit
	}

	list, err := lh.apiProvider.Slack().GetListInfoContext(ctx, listID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	resp, err := lh.apiProvider.Slack().ListsItemsListContext(
		ctx,
		listID,
		limit,
		request.GetString("cursor", ""),
		request.GetBool("include_archived", false),
	)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result := map[string]any{
		"list":  list,
		"items": resp.Items,
	}
	if resp.ResponseMetadata.NextCursor != "" {
		result["next_cursor"] = resp.ResponseMetadata.NextCursor
	}

	return marshalJSONToolResult(result)
}

func (lh *ListsHandler) ListsItemsCreateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if ready, err := lh.apiProvider.IsReady(); !ready {
		lh.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultError(err.Error()), nil
	}

	listID, err := normalizeListID(request.GetString("list_id", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list, err := lh.apiProvider.Slack().GetListInfoContext(ctx, listID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	fieldValues, err := parseFieldValues(request.GetString("field_values", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	initialFields, err := buildListFieldInputs(list.ListMetadata, fieldValues, request.GetString("parent_item_id", ""), "")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	resp, err := lh.apiProvider.Slack().ListsItemsCreateContext(
		ctx,
		listID,
		initialFields,
		request.GetString("parent_item_id", ""),
	)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	createdItem := resp.Item
	if createdItem == nil && len(resp.Items) > 0 {
		createdItem = resp.Items[0]
	}

	return marshalJSONToolResult(map[string]any{
		"list":         list,
		"created_item": createdItem,
	})
}

func (lh *ListsHandler) ListsItemsUpdateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if ready, err := lh.apiProvider.IsReady(); !ready {
		lh.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultError(err.Error()), nil
	}

	listID, err := normalizeListID(request.GetString("list_id", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	itemID := strings.TrimSpace(request.GetString("item_id", ""))
	if itemID == "" {
		return mcp.NewToolResultError("item_id parameter is required"), nil
	}

	list, err := lh.apiProvider.Slack().GetListInfoContext(ctx, listID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	fieldValues, err := parseFieldValues(request.GetString("field_values", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cells, err := buildListFieldInputs(list.ListMetadata, fieldValues, "", itemID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	resp, err := lh.apiProvider.Slack().ListsItemsUpdateContext(ctx, listID, cells)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	updatedItem := resp.Item
	if updatedItem == nil && len(resp.Items) > 0 {
		updatedItem = resp.Items[0]
	}

	return marshalJSONToolResult(map[string]any{
		"list":         list,
		"updated_item": updatedItem,
	})
}

func marshalJSONToolResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(b)), nil
}

func normalizeListID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("list_id parameter is required")
	}
	if strings.HasPrefix(raw, "F") {
		return raw, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("list_id must be a Slack List file ID (F...) or a Slack List URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.HasPrefix(parts[i], "F") {
			return parts[i], nil
		}
	}

	return "", fmt.Errorf("list_id must be a Slack List file ID (F...) or a Slack List URL")
}

func parseFieldValues(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("field_values parameter is required")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("field_values must be a JSON object: %w", err)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("field_values must not be empty")
	}
	return parsed, nil
}

func buildListFieldInputs(metadata *provider.ListMetadata, values map[string]any, parentItemID, rowID string) ([]map[string]any, error) {
	if metadata == nil {
		return nil, fmt.Errorf("Slack List metadata is unavailable")
	}

	schema := metadata.Schema
	if parentItemID != "" && len(metadata.SubtaskSchema) > 0 {
		schema = metadata.SubtaskSchema
	}

	fields := make([]map[string]any, 0, len(values))
	for key, value := range values {
		column, err := resolveListColumn(schema, key)
		if err != nil {
			return nil, err
		}

		field, err := buildListFieldInput(column, value, rowID)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}

	return fields, nil
}

func resolveListColumn(schema []provider.ListColumn, key string) (*provider.ListColumn, error) {
	trimmedKey := strings.TrimSpace(key)
	for i := range schema {
		column := &schema[i]
		if column.ID == trimmedKey || column.Key == trimmedKey {
			return column, nil
		}
	}
	for i := range schema {
		column := &schema[i]
		if strings.EqualFold(column.Name, trimmedKey) || strings.EqualFold(column.Key, trimmedKey) {
			return column, nil
		}
	}
	return nil, fmt.Errorf("unknown list field %q", key)
}

func buildListFieldInput(column *provider.ListColumn, value any, rowID string) (map[string]any, error) {
	field := map[string]any{
		"column_id": column.ID,
	}
	if rowID != "" {
		field["row_id"] = rowID
	}

	switch column.Type {
	case "text":
		richText, err := normalizeRichTextValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", column.Name, err)
		}
		field["rich_text"] = richText
	case "todo_assignee", "user", "channel", "attachment", "email", "phone", "date", "message", "select", "todo_due_date":
		values, err := normalizeStringArrayValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", column.Name, err)
		}
		field[fieldValueKey(column.Type)] = values
	case "checkbox", "todo_completed":
		boolValue, err := normalizeBoolValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", column.Name, err)
		}
		field["checkbox"] = boolValue
	case "number", "rating", "timestamp":
		numbers, err := normalizeNumericArrayValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", column.Name, err)
		}
		field[fieldValueKey(column.Type)] = numbers
	case "link":
		links, err := normalizeLinkValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", column.Name, err)
		}
		field["link"] = links
	default:
		return nil, fmt.Errorf("%s: unsupported list field type %q", column.Name, column.Type)
	}

	return field, nil
}

func fieldValueKey(fieldType string) string {
	switch fieldType {
	case "todo_assignee":
		return "user"
	case "todo_due_date":
		return "date"
	case "todo_completed":
		return "checkbox"
	default:
		return fieldType
	}
}

func normalizeRichTextValue(value any) ([]map[string]any, error) {
	switch v := value.(type) {
	case string:
		return []map[string]any{
			{
				"type": "rich_text",
				"elements": []map[string]any{
					{
						"type": "rich_text_section",
						"elements": []map[string]any{
							{
								"type": "text",
								"text": v,
							},
						},
					},
				},
			},
		}, nil
	case []any:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var blocks []map[string]any
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return nil, fmt.Errorf("text field must be a string or rich_text block array")
		}
		return blocks, nil
	default:
		return nil, fmt.Errorf("text field must be a string or rich_text block array")
	}
}

func normalizeStringArrayValue(value any) ([]string, error) {
	switch v := value.(type) {
	case string:
		return []string{v}, nil
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("value array must contain only strings")
			}
			result = append(result, s)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("value must be a string or array of strings")
	}
}

func normalizeBoolValue(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("value must be a boolean")
		}
		return parsed, nil
	default:
		return false, fmt.Errorf("value must be a boolean")
	}
}

func normalizeNumericArrayValue(value any) ([]float64, error) {
	switch v := value.(type) {
	case float64:
		return []float64{v}, nil
	case int:
		return []float64{float64(v)}, nil
	case int64:
		return []float64{float64(v)}, nil
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("value must be numeric")
		}
		return []float64{parsed}, nil
	case []any:
		result := make([]float64, 0, len(v))
		for _, item := range v {
			values, err := normalizeNumericArrayValue(item)
			if err != nil {
				return nil, err
			}
			result = append(result, values...)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("value must be numeric or an array of numerics")
	}
}

func normalizeLinkValue(value any) ([]map[string]any, error) {
	switch v := value.(type) {
	case string:
		return []map[string]any{
			{
				"original_url":   v,
				"display_as_url": false,
			},
		}, nil
	case map[string]any:
		return []map[string]any{v}, nil
	case []any:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var links []map[string]any
		if err := json.Unmarshal(raw, &links); err != nil {
			return nil, fmt.Errorf("link field must be a string, object, or array of objects")
		}
		return links, nil
	default:
		return nil, fmt.Errorf("link field must be a string, object, or array of objects")
	}
}
