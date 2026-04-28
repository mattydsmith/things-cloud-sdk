# Deployments

## Current environment split

| Environment | Fly app | Hostname | Purpose | Current state |
|-------------|---------|----------|---------|---------------|
| `prod` | `things-cloud-mttsmth` | `https://things-cloud-mttsmth.fly.dev` | Primary day-to-day deployment | Main Things account; `/mcp` and `/api/*` currently open |
| `dev` | `things-cloud-sdk` | `https://things-cloud-sdk.fly.dev` | Branch testing and upcoming web UI work | Separate Things account; `/mcp` and `/api/*` currently open |

## Account separation

Keep `prod` and `dev` on separate Things Cloud accounts.

That gives you a safe end-to-end environment for:

- MCP protocol and auth changes
- REST API changes
- web UI development
- destructive sync and write-path testing

Avoid pointing both Fly apps at the same Things account unless you explicitly want both environments mutating the same data.

## Claude setup

Keep separate Claude connectors or MCP server entries for `prod` and `dev` when you need both environments available. If you only keep one Claude.ai connector, keep it pointed at `prod` and switch to `dev` only intentionally during testing.

### Claude.ai

Create two custom connectors with clear names, for example:

- `Things (prod)` -> `https://things-cloud-mttsmth.fly.dev/mcp`
- `Things (dev)` -> `https://things-cloud-sdk.fly.dev/mcp`

Both current deployments are open on `/mcp` right now. Use the URL only; do not add auth settings.

Do not repoint your production connector at the dev app just to test a branch.

### Claude Code

Example multi-environment config:

```json
{
  "mcpServers": {
    "things-prod": {
      "type": "url",
      "url": "https://things-cloud-mttsmth.fly.dev/mcp"
    },
    "things-dev": {
      "type": "url",
      "url": "https://things-cloud-sdk.fly.dev/mcp"
    }
  }
}
```

If you later enable `API_KEY` for REST-only testing, it still does not affect `/mcp`, so this Claude Code config stays the same.

## Testing against dev

The shell suites accept an explicit `base_url`, so pass the target deployment instead of relying on defaults:

```bash
./tests/test-mcp-protocol.sh https://things-cloud-sdk.fly.dev
./tests/test-mcp-read.sh https://things-cloud-sdk.fly.dev
./tests/test-smoke.sh https://things-cloud-sdk.fly.dev
./tests/test-api.sh https://things-cloud-sdk.fly.dev your-api-key
./tests/test-mcp.sh 010 https://things-cloud-sdk.fly.dev
```

Most scripts still default to the production URL, so always pass the dev base URL explicitly when validating branch-only changes. `test-api.sh` is only relevant when you have explicitly enabled `API_KEY` on the target deployment.
