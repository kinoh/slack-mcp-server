package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListInfoResponseUnmarshalAcceptsObjectOptions(t *testing.T) {
	payload := []byte(`{
		"ok": true,
		"file": {
			"id": "F0AKG9SV864",
			"filetype": "list",
			"list_metadata": {
				"schema": [
					{
						"id": "col_status",
						"name": "Status",
						"type": "select",
						"options": {
							"todo": {"id": "opt_1", "name": "Todo"},
							"done": {"id": "opt_2", "name": "Done"}
						}
					}
				]
			}
		}
	}`)

	var resp listInfoResponse
	err := json.Unmarshal(payload, &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.File)
	require.NotNil(t, resp.File.ListMetadata)
	require.Len(t, resp.File.ListMetadata.Schema, 1)

	options, ok := resp.File.ListMetadata.Schema[0].Options.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, options, "todo")
	assert.Contains(t, options, "done")
}

func TestListInfoResponseUnmarshalAcceptsArrayOptions(t *testing.T) {
	payload := []byte(`{
		"ok": true,
		"file": {
			"id": "F0AKG9SV864",
			"filetype": "list",
			"list_metadata": {
				"schema": [
					{
						"id": "col_status",
						"name": "Status",
						"type": "select",
						"options": [
							{"id": "opt_1", "name": "Todo"},
							{"id": "opt_2", "name": "Done"}
						]
					}
				]
			}
		}
	}`)

	var resp listInfoResponse
	err := json.Unmarshal(payload, &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.File)
	require.NotNil(t, resp.File.ListMetadata)
	require.Len(t, resp.File.ListMetadata.Schema, 1)

	options, ok := resp.File.ListMetadata.Schema[0].Options.([]any)
	require.True(t, ok)
	require.Len(t, options, 2)
}

func TestListsItemsInfoResponseUnmarshal(t *testing.T) {
	payload := []byte(`{
		"ok": true,
		"record": {
			"id": "Rec123",
			"list_id": "F0AKG9SV864",
			"fields": [
				{
					"key": "name",
					"text": "Ship Lists"
				}
			]
		}
	}`)

	var resp ListsItemsInfoResponse
	err := json.Unmarshal(payload, &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Record)
	assert.Equal(t, "Rec123", resp.Record["id"])
	assert.Equal(t, "F0AKG9SV864", resp.Record["list_id"])
}
