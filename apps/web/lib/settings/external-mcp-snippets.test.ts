import { describe, it, expect } from "vitest";
import {
  buildAuggieCliCommand,
  buildAuggieConfig,
  buildClaudeCodeCliCommand,
  buildClaudeCodeConfig,
  buildCodexCliCommand,
  buildCodexConfig,
  buildCopilotCliConfig,
  buildCursorConfig,
  buildOpenCodeConfig,
} from "./external-mcp-snippets";

const URL = "http://localhost:38429/mcp";

describe("external MCP snippets", () => {
  it("Claude Code uses http transport with the streamable URL", () => {
    const snippet = buildClaudeCodeConfig(URL);
    const parsed = JSON.parse(snippet);
    expect(parsed.mcpServers.kandev).toEqual({ type: "http", url: URL });
  });

  it("Claude Code CLI command targets http transport at user scope", () => {
    const command = buildClaudeCodeCliCommand(URL);
    expect(command).toBe(`claude mcp add --transport http --scope user kandev ${URL}`);
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

  it("Codex CLI command targets the streamable URL", () => {
    const command = buildCodexCliCommand(URL);
    expect(command).toBe(`codex mcp add kandev --url ${URL}`);
  });

  it("Auggie settings.json uses http transport with the streamable URL", () => {
    const snippet = buildAuggieConfig(URL);
    const parsed = JSON.parse(snippet);
    expect(parsed.mcpServers.kandev).toEqual({ type: "http", url: URL });
  });

  it("Auggie CLI command targets http transport with the streamable URL", () => {
    const command = buildAuggieCliCommand(URL);
    expect(command).toBe(`auggie mcp add kandev --transport http --url ${URL}`);
  });

  it("OpenCode opencode.json uses remote transport and is enabled by default", () => {
    const snippet = buildOpenCodeConfig(URL);
    const parsed = JSON.parse(snippet);
    expect(parsed.$schema).toBe("https://opencode.ai/config.json");
    expect(parsed.mcp.kandev).toEqual({ type: "remote", url: URL, enabled: true });
  });

  it("Copilot CLI mcp-config.json uses http transport and exposes all tools", () => {
    const snippet = buildCopilotCliConfig(URL);
    const parsed = JSON.parse(snippet);
    expect(parsed.mcpServers.kandev).toEqual({ type: "http", url: URL, tools: ["*"] });
  });

  it("snippets include the exact URL that was passed in", () => {
    const customUrl = "http://192.168.1.10:55555/mcp";
    expect(buildClaudeCodeConfig(customUrl)).toContain(customUrl);
    expect(buildClaudeCodeCliCommand(customUrl)).toContain(customUrl);
    expect(buildCursorConfig(customUrl)).toContain(customUrl);
    expect(buildCodexConfig(customUrl)).toContain(customUrl);
    expect(buildCodexCliCommand(customUrl)).toContain(customUrl);
    expect(buildAuggieConfig(customUrl)).toContain(customUrl);
    expect(buildAuggieCliCommand(customUrl)).toContain(customUrl);
    expect(buildOpenCodeConfig(customUrl)).toContain(customUrl);
    expect(buildCopilotCliConfig(customUrl)).toContain(customUrl);
  });
});
