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
