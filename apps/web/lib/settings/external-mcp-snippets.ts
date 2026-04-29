// Builds ready-to-paste MCP server config snippets for popular external coding agents.
// Each snippet wires the agent to Kandev's external MCP endpoint.

const SERVER_NAME = "kandev";

export function buildClaudeCodeConfig(streamableUrl: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        [SERVER_NAME]: {
          type: "http",
          url: streamableUrl,
        },
      },
    },
    null,
    2,
  );
}

export function buildClaudeCodeCliCommand(streamableUrl: string): string {
  return `claude mcp add --transport http --scope user ${SERVER_NAME} ${streamableUrl}`;
}

export function buildCursorConfig(streamableUrl: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        [SERVER_NAME]: {
          url: streamableUrl,
        },
      },
    },
    null,
    2,
  );
}

export function buildCodexConfig(streamableUrl: string): string {
  return [
    `[mcp_servers.${SERVER_NAME}]`,
    `url = "${streamableUrl}"`,
    `transport = "streamable_http"`,
  ].join("\n");
}

export function buildCodexCliCommand(streamableUrl: string): string {
  return `codex mcp add ${SERVER_NAME} --url ${streamableUrl}`;
}

// Auggie's ~/.augment/settings.json uses the same { type: "http", url } shape as Claude Code.
export function buildAuggieConfig(streamableUrl: string): string {
  return buildClaudeCodeConfig(streamableUrl);
}

export function buildAuggieCliCommand(streamableUrl: string): string {
  return `auggie mcp add ${SERVER_NAME} --transport http --url ${streamableUrl}`;
}

export function buildOpenCodeConfig(streamableUrl: string): string {
  return JSON.stringify(
    {
      $schema: "https://opencode.ai/config.json",
      mcp: {
        [SERVER_NAME]: {
          type: "remote",
          url: streamableUrl,
          enabled: true,
        },
      },
    },
    null,
    2,
  );
}

export function buildCopilotCliConfig(streamableUrl: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        [SERVER_NAME]: {
          type: "http",
          url: streamableUrl,
          tools: ["*"],
        },
      },
    },
    null,
    2,
  );
}
