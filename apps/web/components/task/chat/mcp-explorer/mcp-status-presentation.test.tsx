import { describe, expect, it } from "vitest";
import { MCP_STATUS_DOT_CLASS } from "./mcp-status-presentation";

// @covers AC-AGENTS-MCP-MANAGEMENT-003.2
// This shared map feeds tooltip, list and detail evidence, independently of selections.
describe("MCP evidence colors", () => {
  it("keeps delivery and absent observation neutral", () => {
    expect(MCP_STATUS_DOT_CLASS.delivered).toBe("bg-muted-foreground/50");
    expect(MCP_STATUS_DOT_CLASS.unknown).toBe(MCP_STATUS_DOT_CLASS.delivered);
  });

  it("retains stronger observed connection and failure meanings", () => {
    expect(MCP_STATUS_DOT_CLASS.active).toBe("bg-emerald-500");
    expect(MCP_STATUS_DOT_CLASS.connected).toBe("bg-amber-500");
    expect(MCP_STATUS_DOT_CLASS.failed).toBe("bg-destructive");
  });
});
