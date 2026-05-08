# Tools Reference

This page lists the MCP tools exposed by this fork. Tool availability still depends on token type, `SLACK_MCP_ENABLED_TOOLS`, and tool-specific safety environment variables.

## Conversations

### `conversations_history`

Gets messages from a channel or DM.

Parameters:
- `channel_id` (string, required): Channel ID, channel name such as `#general`, or DM lookup such as `@username_dm`.
- `include_activity_messages` (boolean, default `false`): Include activity messages such as joins and leaves.
- `cursor` (string): Pagination cursor returned by the previous response.
- `limit` (string, default `1d`): Time range such as `1d`, `1w`, `30d`, `90d`, or a message count such as `50`.

### `conversations_replies`

Gets replies in a thread.

Parameters:
- `channel_id` (string, required): Channel ID, channel name, or DM lookup.
- `thread_ts` (string, required): Thread parent timestamp in `1234567890.123456` format.
- `include_activity_messages` (boolean, default `false`): Include activity messages.
- `cursor` (string): Pagination cursor returned by the previous response.
- `limit` (string, default `1d`): Time range or message count.

### `conversations_search_messages`

Searches Slack messages. This tool is not registered for bot tokens because Slack bot tokens cannot use `search.messages`.

Parameters:
- `search_query` (string): Free-text Slack search query or a Slack message URL.
- `filter_in_channel` (string): Channel ID or channel name.
- `filter_in_im_or_mpim` (string): DM or MPIM ID or lookup name.
- `filter_users_with` (string): User ID or display name for messages involving a user.
- `filter_users_from` (string): User ID or display name for message author.
- `filter_date_before` (string): Date filter such as `2023-10-01`, `July`, `Yesterday`, or `Today`.
- `filter_date_after` (string): Date filter.
- `filter_date_on` (string): Date filter.
- `filter_date_during` (string): Date period filter.
- `filter_threads_only` (boolean, default `false`): Return only thread messages.
- `cursor` (string): Pagination cursor returned by the previous response.
- `limit` (number, default `20`): Maximum number of items, from 1 to 100.

### `conversations_unreads`

Gets unread messages across channels. This tool is not registered for bot tokens.

Parameters:
- `include_messages` (boolean, default `true`): Include unread message bodies instead of only summaries.
- `channel_types` (string, default `all`): `all`, `dm`, `group_dm`, `partner`, or `internal`.
- `max_channels` (number, default `50`): Maximum number of channels to inspect.
- `max_messages_per_channel` (number, default `10`): Maximum unread messages per channel.
- `mentions_only` (boolean, default `false`): Return only channels with mentions.
- `include_muted` (boolean, default `false`): Include muted channels.

### `conversations_mark`

Marks a channel or DM as read.

Parameters:
- `channel_id` (string, required): Channel ID, channel name, or DM lookup.
- `ts` (string): Timestamp to mark read up to. If omitted, marks all messages as read.

### `conversations_add_message`

Posts a message. This write tool is disabled by default and requires `SLACK_MCP_ADD_MESSAGE_TOOL` or explicit registration through `SLACK_MCP_ENABLED_TOOLS`.

Parameters:
- `channel_id` (string, required): Channel ID, channel name, or DM lookup.
- `thread_ts` (string): Thread timestamp. If omitted, posts to the channel.
- `text` (string): Message text.
- `content_type` (string, default `text/markdown`): `text/markdown` or `text/plain`.

## Channels And Users

### `channels_list`

Lists Slack channels.

Parameters:
- `channel_types` (string, required): Comma-separated values from `mpim`, `im`, `public_channel`, and `private_channel`.
- `sort` (string): `popularity` sorts by member count.
- `limit` (number, default `100`): Maximum number of items, up to 999.
- `cursor` (string): Pagination cursor returned by the previous response.

### `users_search`

Searches users by name, display name, username, or email.

Parameters:
- `query` (string, required): Search query.
- `limit` (number, default `10`): Maximum number of results, from 1 to 100.

## Files, Canvases, And Lists Search

### `search_files_and_canvases`

Searches Slack files, canvases, and Slack Lists using `search.files`. Returned Slack List file IDs can be passed to `lists_items_*` tools.

Parameters:
- `search_query` (string): Free-text Slack file search query.
- `filter_in_channel` (string): Channel ID or channel name.
- `filter_users_from` (string): Uploader user ID or display name.
- `filter_date_before` (string): File creation date filter.
- `filter_date_after` (string): File creation date filter.
- `cursor` (string): Pagination cursor returned by the previous response.
- `limit` (number, default `20`): Maximum number of items, from 1 to 100.

### `attachment_get_data`

Downloads an attachment by Slack file ID. This write-sensitive data access tool is disabled by default and requires `SLACK_MCP_ATTACHMENT_TOOL` or explicit registration through `SLACK_MCP_ENABLED_TOOLS`.

Parameters:
- `file_id` (string, required): Slack file ID such as `F1234567890`.

## Canvas Tools

Canvas support was integrated from PR #123 and then documented for section lookup usage in this fork.

### `canvases_create`

Creates a Slack Canvas with markdown content.

Parameters:
- `title` (string): Canvas title.
- `content` (string, required): Markdown content.

### `canvases_read`

Reads Slack Canvas metadata and content.

Parameters:
- `canvas_id` (string, required): Slack Canvas file ID such as `F1234567890`.

### `canvases_edit`

Edits an existing Slack Canvas.

Parameters:
- `canvas_id` (string, required): Slack Canvas file ID.
- `operation` (string, default `insert_at_end`): `insert_at_start`, `insert_at_end`, `insert_before`, `insert_after`, `replace`, or `delete`.
- `content` (string, required): Markdown content to add or use as replacement.
- `section_id` (string): Required for `insert_before`, `insert_after`, and `delete`. Use `canvases_sections_lookup` to find it.

### `canvases_sections_lookup`

Finds section IDs for targeted Canvas edits.

Parameters:
- `canvas_id` (string, required): Slack Canvas file ID.
- `contains_text` (string): Short phrase to match inside section content.

Typical flow: read the canvas, identify nearby text, call `canvases_sections_lookup`, then pass the returned `section_id` to `canvases_edit`.

## Slack Lists

Slack Lists tools are registered only for OAuth tokens (`xoxp` or `xoxb`). They use Slack's official Lists API.

### `lists_items_list`

Lists items in a Slack List and returns list metadata with item data.

Parameters:
- `list_id` (string, required): Slack List file ID such as `F1234567890` or a Slack List URL.
- `limit` (number, default `50`): Maximum number of items, from 1 to 100.
- `cursor` (string): Cursor returned as `next_cursor`.
- `include_archived` (boolean, default `false`): Include archived items.

### `lists_items_create`

Creates a Slack List item.

Parameters:
- `list_id` (string, required): Slack List file ID or URL.
- `field_values` (string, required): JSON object keyed by field ID, field key, or field name.
- `parent_item_id` (string): Parent item ID for sub-items.

### `lists_items_update`

Updates a Slack List item.

Parameters:
- `list_id` (string, required): Slack List file ID or URL.
- `item_id` (string, required): Slack List item ID.
- `field_values` (string, required): JSON object keyed by field ID, field key, or field name.

Constraints:
- Required scopes are `lists:read`, `lists:write`, and `files:read`.
- `files:read` is used to resolve list metadata through `files.info`.
- There is no separate list discovery tool. Use `search_files_and_canvases` to find Lists, then pass the returned `F...` ID here.
- Complex field values must be Slack API compatible, such as option IDs, user IDs, channel IDs, link objects, or rich text compatible values.
- Unsupported field types return explicit errors instead of being silently coerced.

## Reactions

### `reactions_add`

Adds an emoji reaction. This write tool is disabled by default and requires `SLACK_MCP_REACTION_TOOL` or explicit registration through `SLACK_MCP_ENABLED_TOOLS`.

Parameters:
- `channel_id` (string, required): Channel ID, channel name, or DM lookup.
- `timestamp` (string, required): Message timestamp.
- `emoji` (string, required): Emoji name without colons.

### `reactions_remove`

Removes an emoji reaction. This write tool is disabled by default and requires `SLACK_MCP_REACTION_TOOL` or explicit registration through `SLACK_MCP_ENABLED_TOOLS`.

Parameters:
- `channel_id` (string, required): Channel ID, channel name, or DM lookup.
- `timestamp` (string, required): Message timestamp.
- `emoji` (string, required): Emoji name without colons.

## User Groups

### `usergroups_list`

Lists Slack user groups.

Parameters:
- `include_users` (boolean, default `false`): Include member user IDs.
- `include_count` (boolean, default `true`): Include user counts.
- `include_disabled` (boolean, default `false`): Include disabled groups.

### `usergroups_me`

Manages the authenticated user's user group membership.

Parameters:
- `action` (string, required): `list`, `join`, or `leave`.
- `usergroup_id` (string): Required for `join` and `leave`.

### `usergroups_create`

Creates a user group.

Parameters:
- `name` (string, required): Display name.
- `handle` (string): Mention handle without `@`.
- `description` (string): Group description.
- `channels` (string): Comma-separated default channel IDs.

### `usergroups_update`

Updates user group metadata.

Parameters:
- `usergroup_id` (string, required): User group ID.
- `name` (string): New display name.
- `handle` (string): New mention handle without `@`.
- `description` (string): New description.
- `channels` (string): Replacement comma-separated default channel IDs.

### `usergroups_users_update`

Replaces all members of a user group.

Parameters:
- `usergroup_id` (string, required): User group ID.
- `users` (string, required): Comma-separated complete member list of user IDs.

## Resources

### `slack://<workspace>/channels`

Returns a CSV directory of channels.

Fields:
- `id`
- `name`
- `topic`
- `purpose`
- `memberCount`

### `slack://<workspace>/users`

Returns a CSV directory of users.

Fields:
- `userID`
- `userName`
- `realName`
