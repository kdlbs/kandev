import { describe, it, expect } from "vitest";
import {
  buildClaudeCodeConfig,
  buildCodexConfig,
  buildCursorConfig,
} from "./external-mcp-snippets";

const URL = "http://localhost:38429/mcp";

describe("external MCP snippets", () => {
  it("Claude Code uses http transport with the streamable URL", () => {
    const snippet = buildClaudeCodeConfig(URL);
    const parsed = JSON.parse(snippet);
    expect(parsed.mcpServers.kandev).toEqual({ type: "http", url: URL });
  });

  it("Cursor wires the URL under mcpServers", () => {
    const snippet = buildCursorConfig(URL);
    const parsed = JSON.parse(snippet);
    expect(parsed.mcpServers.kandev).toEqual({ url: URL });
  });

  it("Codex emits a TOML block with streamable_http transport", () => {
    const snippet = buildCodexConfig(URL);
    expect(snippet).toContain("[mcp_servers.kandev]");
    expect(snippet).toContain(`url = "${URL}"`);
    expect(snippet).toContain(`transport = "streamable_http"`);
  });

  it("snippets include the exact URL that was passed in", () => {
    const customUrl = "http://192.168.1.10:55555/mcp";
    expect(buildClaudeCodeConfig(customUrl)).toContain(customUrl);
    expect(buildCursorConfig(customUrl)).toContain(customUrl);
    expect(buildCodexConfig(customUrl)).toContain(customUrl);
  });
});
