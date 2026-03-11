package handler

import (
	"testing"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeListID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "raw file id",
			input: "F1234567890",
			want:  "F1234567890",
		},
		{
			name:  "workspace url",
			input: "https://app.slack.com/client/T123/lists/T123/F1234567890",
			want:  "F1234567890",
		},
		{
			name:  "workspace subdomain url",
			input: "https://example.slack.com/lists/T123/F1234567890",
			want:  "F1234567890",
		},
		{
			name:    "invalid",
			input:   "not-a-list",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeListID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildListFieldInputs(t *testing.T) {
	metadata := &provider.ListMetadata{
		Schema: []provider.ListColumn{
			{ID: "col_title", Key: "title", Name: "Title", Type: "text"},
			{ID: "col_status", Key: "status", Name: "Status", Type: "select"},
			{ID: "col_done", Key: "todo_completed", Name: "Done", Type: "todo_completed"},
			{ID: "col_due", Key: "todo_due_date", Name: "Due", Type: "todo_due_date"},
			{ID: "col_points", Key: "points", Name: "Points", Type: "number"},
			{ID: "col_link", Key: "link", Name: "Link", Type: "link"},
		},
	}

	fields, err := buildListFieldInputs(metadata, map[string]any{
		"Title":  "Ship Lists",
		"status": "In Progress",
		"Done":   true,
		"Due":    "2026-03-31",
		"Points": 3,
		"Link":   "https://example.com/spec",
	}, "", "row_1")
	require.NoError(t, err)
	require.Len(t, fields, 6)

	assert.Contains(t, fields, map[string]any{
		"column_id": "col_title",
		"row_id":    "row_1",
		"rich_text": []map[string]any{
			{
				"type": "rich_text",
				"elements": []map[string]any{
					{
						"type": "rich_text_section",
						"elements": []map[string]any{
							{
								"type": "text",
								"text": "Ship Lists",
							},
						},
					},
				},
			},
		},
	})
	assert.Contains(t, fields, map[string]any{
		"column_id": "col_status",
		"row_id":    "row_1",
		"select":    []string{"In Progress"},
	})
	assert.Contains(t, fields, map[string]any{
		"column_id": "col_done",
		"row_id":    "row_1",
		"checkbox":  true,
	})
	assert.Contains(t, fields, map[string]any{
		"column_id": "col_due",
		"row_id":    "row_1",
		"date":      []string{"2026-03-31"},
	})
	assert.Contains(t, fields, map[string]any{
		"column_id": "col_points",
		"row_id":    "row_1",
		"number":    []float64{3},
	})
}

func TestBuildListFieldInputsRejectsUnsupportedType(t *testing.T) {
	metadata := &provider.ListMetadata{
		Schema: []provider.ListColumn{
			{ID: "col_ref", Name: "Reference", Type: "reference"},
		},
	}

	_, err := buildListFieldInputs(metadata, map[string]any{
		"Reference": "F1234567890",
	}, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported list field type")
}
