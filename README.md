# Slack MCP Server Fork

This repository is a fork of [korotovsky/slack-mcp-server](https://github.com/korotovsky/slack-mcp-server).

It tracks upstream while carrying a small set of Slack-workflow focused changes:

- Canvas operations from PR #123 are integrated and documented for create, read, edit, and section lookup flows.
- File, Canvas, and Slack List search is exposed through `search_files_and_canvases`.
- Search results include safer filtering behavior for channel cache usage.
- Slack Lists item tools are available for OAuth tokens, including bot tokens where the Slack API supports them.
- Slack List field values are normalized for Slack's rich text field format, and list metadata is returned with item responses.

For upstream behavior and project background, refer to the original repository. This README only describes the fork-level orientation.

## Documentation

- [Authentication Setup](docs/01-authentication-setup.md)
- [Installation](docs/02-installation.md)
- [Configuration and Usage](docs/03-configuration-and-usage.md)
- [Tools Reference](docs/04-tools.md)

## License

Licensed under MIT - see [LICENSE](LICENSE). This is not an official Slack product.
