package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/server/auth"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

const (
	huddleTranscriptFiletype = "huddle_transcript"
	huddleTranscriptMimetype = "application/vnd.slack-huddle-transcript"
)

var slackFileIDPattern = regexp.MustCompile(`^F[A-Z0-9]+$`)

type HuddleTranscriptsHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

type huddleTranscriptSegment struct {
	Offset    string `json:"offset,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Speaker   string `json:"speaker,omitempty"`
	Text      string `json:"text"`
}

func NewHuddleTranscriptsHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *HuddleTranscriptsHandler {
	return &HuddleTranscriptsHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

func (hh *HuddleTranscriptsHandler) HuddleTranscriptReadHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	hh.logger.Debug("HuddleTranscriptReadHandler called", zap.Any("params", request.Params.Arguments))

	if authenticated, err := auth.IsAuthenticated(ctx, hh.apiProvider.ServerTransport(), hh.logger); !authenticated {
		hh.logger.Error("Authentication failed for huddle_transcript_read", zap.Error(err))
		return mcp.NewToolResultError(err.Error()), nil
	}

	if ready, err := hh.apiProvider.IsReady(); !ready {
		hh.logger.Error("API provider not ready", zap.Error(err))
		return mcp.NewToolResultError(err.Error()), nil
	}

	input := strings.TrimSpace(request.GetString("url_or_file_id", ""))
	if input == "" {
		return huddleTranscriptResult(map[string]any{
			"ok":     false,
			"reason": "invalid_input",
			"error":  "url_or_file_id parameter is required",
		})
	}

	sourceID, err := normalizeSlackFileID(input)
	if err != nil {
		return huddleTranscriptResult(map[string]any{
			"ok":     false,
			"reason": "invalid_input",
			"error":  err.Error(),
		})
	}

	resp, err := hh.apiProvider.Slack().GetFileInfoRawContext(ctx, sourceID, provider.WithHuddleTranscription())
	if err != nil {
		hh.logger.Error("files.info failed for huddle transcript read", zap.String("file_id", sourceID), zap.Error(err))
		return huddleTranscriptResult(map[string]any{
			"ok":     false,
			"reason": "api_error",
			"error":  err.Error(),
		})
	}
	if !resp.Ok {
		return huddleTranscriptResult(slackFileErrorResult(resp))
	}

	file := resp.File
	if isHuddleTranscriptFile(file) {
		result, err := hh.readTranscriptFile(ctx, sourceID, "", file, resp.Raw)
		if err != nil {
			hh.logger.Warn("Failed to read huddle transcript file", zap.String("file_id", sourceID), zap.Error(err))
		}
		return huddleTranscriptResult(result)
	}

	transcriptFileID, err := extractEmbeddedTranscriptFileID(file)
	if err != nil {
		return huddleTranscriptResult(map[string]any{
			"ok":                false,
			"reason":            "transcript_file_not_found",
			"source_canvas_id":  sourceID,
			"filetype":          stringFromMap(file, "filetype"),
			"mimetype":          stringFromMap(file, "mimetype"),
			"error":             err.Error(),
			"raw_file_metadata": file,
		})
	}

	transcriptResp, err := hh.apiProvider.Slack().GetFileInfoRawContext(ctx, transcriptFileID, provider.WithHuddleTranscription())
	if err != nil {
		hh.logger.Error("files.info failed for embedded huddle transcript", zap.String("source_canvas_id", sourceID), zap.String("transcript_file_id", transcriptFileID), zap.Error(err))
		return huddleTranscriptResult(map[string]any{
			"ok":                 false,
			"reason":             "api_error",
			"source_canvas_id":   sourceID,
			"transcript_file_id": transcriptFileID,
			"error":              err.Error(),
		})
	}
	if !transcriptResp.Ok {
		result := slackFileErrorResult(transcriptResp)
		result["source_canvas_id"] = sourceID
		result["transcript_file_id"] = transcriptFileID
		return huddleTranscriptResult(result)
	}

	result, err := hh.readTranscriptFile(ctx, transcriptFileID, sourceID, transcriptResp.File, transcriptResp.Raw)
	if err != nil {
		hh.logger.Warn("Failed to read embedded huddle transcript", zap.String("source_canvas_id", sourceID), zap.String("transcript_file_id", transcriptFileID), zap.Error(err))
	}
	return huddleTranscriptResult(result)
}

func (hh *HuddleTranscriptsHandler) readTranscriptFile(ctx context.Context, transcriptFileID, sourceCanvasID string, file map[string]any, raw map[string]any) (map[string]any, error) {
	result := map[string]any{
		"ok":                 false,
		"kind":               "huddle_transcript",
		"source_canvas_id":   sourceCanvasID,
		"transcript_file_id": transcriptFileID,
		"filetype":           stringFromMap(file, "filetype"),
		"mimetype":           stringFromMap(file, "mimetype"),
		"created":            int64FromMap(file, "created"),
		"huddle_channel_id":  firstStringFromMap(file, "huddle_channel_id", "channel_id"),
		"huddle_thread_ts":   firstStringFromMap(file, "huddle_thread_ts", "thread_ts"),
		"raw_file_metadata":  file,
	}

	if !isHuddleTranscriptFile(file) {
		result["reason"] = "not_huddle_transcript"
		result["error"] = fmt.Sprintf("%q is not a huddle transcript file", transcriptFileID)
		return result, errors.New("not a huddle transcript file")
	}

	if raw != nil {
		rawFile, _ := raw["file"].(map[string]any)
		if huddleTranscription, ok := rawFile["huddle_transcription"]; ok {
			segments, err := parseTranscriptValue(huddleTranscription)
			if err != nil {
				result["reason"] = "parse_failed"
				result["error"] = err.Error()
				return result, err
			}
			result["ok"] = true
			result["segments"] = segments
			return result, nil
		}
	}

	downloadURL := firstStringFromMap(file, "url_private_download", "url_private")
	if downloadURL == "" {
		result["reason"] = "api_unsupported"
		result["error"] = "files.info did not include huddle_transcription or a private download URL"
		return result, errors.New("no transcript body source")
	}

	body, contentType, err := hh.downloadTranscriptBody(ctx, downloadURL)
	if err != nil {
		result["reason"] = classifyDownloadError(err)
		result["error"] = err.Error()
		return result, err
	}
	if looksLikeHTML(body, contentType) {
		err := errors.New("download returned an HTML shell instead of transcript content")
		result["reason"] = "unexpected_html"
		result["error"] = err.Error()
		return result, err
	}

	segments, err := parseTranscriptBody(body)
	if err != nil {
		result["reason"] = "parse_failed"
		result["error"] = err.Error()
		return result, err
	}
	result["ok"] = true
	result["segments"] = segments
	return result, nil
}

func (hh *HuddleTranscriptsHandler) downloadTranscriptBody(ctx context.Context, downloadURL string) ([]byte, string, error) {
	var buf bytes.Buffer
	err := hh.apiProvider.Slack().GetFileContext(ctx, downloadURL, &buf)
	if err == nil {
		return buf.Bytes(), "", nil
	}
	return nil, "", err
}

func normalizeSlackFileID(input string) (string, error) {
	if slackFileIDPattern.MatchString(input) {
		return input, nil
	}

	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Slack file ID or URL: %q", input)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("Slack URL must use https: %q", input)
	}
	if !isSlackHost(parsed.Hostname()) {
		return "", fmt.Errorf("URL host is not a Slack host: %q", parsed.Hostname())
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if slackFileIDPattern.MatchString(parts[i]) {
			return parts[i], nil
		}
	}
	return "", fmt.Errorf("Slack URL does not contain a file ID: %q", input)
}

func isSlackHost(host string) bool {
	return host == "slack.com" || strings.HasSuffix(host, ".slack.com")
}

func isHuddleTranscriptFile(file map[string]any) bool {
	return stringFromMap(file, "filetype") == huddleTranscriptFiletype ||
		stringFromMap(file, "mimetype") == huddleTranscriptMimetype
}

func extractEmbeddedTranscriptFileID(file map[string]any) (string, error) {
	candidates := []any{
		file["huddle_transcript_file_id"],
		file["transcript_file_id"],
		file["huddle_transcript"],
		file["preview"],
		file["preview_highlight"],
		file["content"],
	}
	for _, candidate := range candidates {
		if id := findHuddleTranscriptID(candidate); id != "" {
			return id, nil
		}
	}
	return "", errors.New("no embedded huddle transcript file ID found in file metadata")
}

func findHuddleTranscriptID(value any) string {
	switch v := value.(type) {
	case string:
		return extractFileIDFromString(v)
	case map[string]any:
		if isHuddleTranscriptFile(v) {
			if id := stringFromMap(v, "id"); slackFileIDPattern.MatchString(id) {
				return id
			}
		}
		for _, nested := range v {
			if id := findHuddleTranscriptID(nested); id != "" {
				return id
			}
		}
	case []any:
		for _, nested := range v {
			if id := findHuddleTranscriptID(nested); id != "" {
				return id
			}
		}
	}
	return ""
}

func extractFileIDFromString(s string) string {
	for _, match := range regexp.MustCompile(`(?:sf:)?(F[A-Z0-9]+)`).FindAllStringSubmatch(s, -1) {
		if len(match) == 2 {
			return match[1]
		}
	}
	return ""
}

func parseTranscriptBody(body []byte) ([]huddleTranscriptSegment, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, errors.New("transcript body is empty")
	}

	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err == nil {
		return parseTranscriptValue(decoded)
	}
	return parsePlainTranscript(string(trimmed))
}

func parseTranscriptValue(value any) ([]huddleTranscriptSegment, error) {
	switch v := value.(type) {
	case []any:
		return parseTranscriptArray(v)
	case map[string]any:
		if blocks, ok := v["blocks"]; ok {
			if segments, err := parseRichTextTranscriptBlocks(blocks); err == nil {
				return segments, nil
			}
		}
		for _, key := range []string{"segments", "utterances", "items", "transcripts", "transcript", "lines"} {
			if nested, ok := v[key]; ok {
				return parseTranscriptValue(nested)
			}
		}
		segment := segmentFromMap(v)
		if segment.Text != "" {
			return []huddleTranscriptSegment{segment}, nil
		}
	case string:
		return parsePlainTranscript(v)
	}
	return nil, errors.New("unsupported transcript structure")
}

func parseTranscriptArray(values []any) ([]huddleTranscriptSegment, error) {
	if segments, err := parseRichTextTranscriptBlocks(values); err == nil {
		return segments, nil
	}

	segments := make([]huddleTranscriptSegment, 0, len(values))
	for _, value := range values {
		switch v := value.(type) {
		case map[string]any:
			segment := segmentFromMap(v)
			if segment.Text != "" {
				segments = append(segments, segment)
			}
		case string:
			plainSegments, err := parsePlainTranscript(v)
			if err == nil {
				segments = append(segments, plainSegments...)
			}
		}
	}
	if len(segments) == 0 {
		return nil, errors.New("no transcript segments found")
	}
	return segments, nil
}

func segmentFromMap(item map[string]any) huddleTranscriptSegment {
	offset := firstStringFromMap(item, "offset", "offset_text", "start_offset")
	if offset == "" {
		if startTimeMS, ok := int64FromMapOK(item, "start_time_ms"); ok {
			offset = formatTranscriptOffsetMS(startTimeMS)
		}
	}
	return huddleTranscriptSegment{
		Offset:    offset,
		Timestamp: firstStringFromMap(item, "timestamp", "ts", "start_time", "start"),
		UserID:    firstStringFromMap(item, "user_id", "user", "speaker_user_id"),
		Speaker:   firstStringFromMap(item, "speaker", "speaker_name", "name"),
		Text:      firstStringFromMap(item, "text", "content", "contents", "transcript"),
	}
}

func parseRichTextTranscriptBlocks(value any) ([]huddleTranscriptSegment, error) {
	segments := make([]huddleTranscriptSegment, 0)
	parseRichTextTranscriptNode(value, &segments)
	if len(segments) == 0 {
		return nil, errors.New("no transcript segments found in rich text blocks")
	}
	return segments, nil
}

func parseRichTextTranscriptNode(value any, segments *[]huddleTranscriptSegment) {
	switch v := value.(type) {
	case []any:
		for _, nested := range v {
			parseRichTextTranscriptNode(nested, segments)
		}
	case map[string]any:
		if stringFromMap(v, "type") == "rich_text_section" {
			if segment := segmentFromRichTextSection(v); segment.Text != "" {
				*segments = append(*segments, segment)
			}
			return
		}
		if elements, ok := v["elements"]; ok {
			parseRichTextTranscriptNode(elements, segments)
		}
	}
}

func segmentFromRichTextSection(section map[string]any) huddleTranscriptSegment {
	elements, _ := section["elements"].([]any)
	segment := huddleTranscriptSegment{}
	var textParts []string
	for _, element := range elements {
		elementMap, ok := element.(map[string]any)
		if !ok {
			continue
		}
		switch stringFromMap(elementMap, "type") {
		case "user":
			if segment.UserID == "" {
				segment.UserID = stringFromMap(elementMap, "user_id")
			}
		case "text":
			text := stringFromMap(elementMap, "text")
			if offset := extractTranscriptOffsetMarker(text); offset != "" {
				segment.Offset = offset
				continue
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				textParts = append(textParts, trimmed)
			}
		}
	}
	segment.Text = strings.TrimSpace(strings.Join(textParts, ""))
	return segment
}

func extractTranscriptOffsetMarker(text string) string {
	match := regexp.MustCompile(`^\s*\[(\d{1,2}:\d{2}(?::\d{2})?)\]:\s*$`).FindStringSubmatch(text)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func formatTranscriptOffsetMS(milliseconds int64) string {
	if milliseconds < 0 {
		milliseconds = 0
	}
	totalSeconds := milliseconds / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func parsePlainTranscript(text string) ([]huddleTranscriptSegment, error) {
	lines := strings.Split(text, "\n")
	segments := make([]huddleTranscriptSegment, 0, len(lines))
	linePattern := regexp.MustCompile(`^\s*(?:\[?(\d{1,2}:\d{2}(?::\d{2})?)\]?\s+)?(?:(U[A-Z0-9]+|[^:]{1,80}):\s+)?(.+?)\s*$`)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := linePattern.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		segment := huddleTranscriptSegment{Offset: match[1], Text: match[3]}
		if strings.HasPrefix(match[2], "U") {
			segment.UserID = match[2]
		} else {
			segment.Speaker = match[2]
		}
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return nil, errors.New("plain transcript contains no parseable lines")
	}
	return segments, nil
}

func looksLikeHTML(body []byte, contentType string) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}
	prefix := strings.ToLower(string(bytes.TrimSpace(body)))
	return strings.HasPrefix(prefix, "<!doctype html") || strings.HasPrefix(prefix, "<html") || strings.Contains(prefix[:min(len(prefix), 512)], "<title>slack")
}

func classifyDownloadError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "missing_scope"):
		return "missing_scope"
	case strings.Contains(msg, "file_not_found"):
		return "file_not_found"
	case strings.Contains(msg, "not_in_channel"):
		return "not_in_channel"
	case strings.Contains(msg, "not_visible"):
		return "not_visible"
	case strings.Contains(msg, "access_denied"), strings.Contains(msg, "403"):
		return "access_denied"
	default:
		return "download_failed"
	}
}

func slackFileErrorResult(resp *provider.FileInfoRawResponse) map[string]any {
	reason := resp.Error
	if reason == "" {
		reason = "api_error"
	}
	return map[string]any{
		"ok":       false,
		"reason":   reason,
		"error":    resp.Error,
		"needed":   resp.Needed,
		"provided": resp.Provided,
	}
}

func huddleTranscriptResult(result map[string]any) (*mcp.CallToolResult, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(resultJSON)), nil
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func firstStringFromMap(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringFromMap(values, key); value != "" {
			return value
		}
	}
	return ""
}

func int64FromMap(values map[string]any, key string) int64 {
	value, ok := int64FromMapOK(values, key)
	if !ok {
		return 0
	}
	return value
}

func int64FromMapOK(values map[string]any, key string) (int64, bool) {
	value, ok := values[key]
	if !ok || value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		i, err := v.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}
