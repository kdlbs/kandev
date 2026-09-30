import { describe, expect, it } from "vitest";
import { agentMcpFailureLabelKey } from "./agent-mcp-prepare-actions";

describe("agent MCP recovery feedback", () => {
  it("explains that recovery must wait for the current agent turn", () => {
    expect(agentMcpFailureLabelKey("session_busy")).toBe("task:agentMcpSessionBusy");
  });

  it("explains when the agent cannot reload MCP servers in the current session", () => {
    expect(agentMcpFailureLabelKey("session_reload_unsupported")).toBe(
      "task:agentMcpSessionReloadUnsupported",
    );
  });
});
