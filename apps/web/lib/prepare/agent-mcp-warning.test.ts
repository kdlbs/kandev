import { describe, expect, it } from "vitest";
import { isAgentMcpAuthWarning } from "./agent-mcp-warning";

describe("isAgentMcpAuthWarning", () => {
  it("identifies failed agent_mcp_verification with authentication_required as an auth warning", () => {
    expect(
      isAgentMcpAuthWarning({
        kind: "agent_mcp_verification",
        status: "failed",
        failureCode: "authentication_required",
      }),
    ).toBe(true);
  });

  it("does not classify non-verification agent MCP steps as auth warnings", () => {
    expect(
      isAgentMcpAuthWarning({
        kind: "agent_mcp_discovery",
        status: "failed",
        failureCode: "authentication_required",
      }),
    ).toBe(false);
    expect(
      isAgentMcpAuthWarning({
        kind: "agent_mcp_approval",
        status: "failed",
        failureCode: "authentication_required",
      }),
    ).toBe(false);
  });

  it("does not classify non-auth verification failure codes as auth warnings", () => {
    expect(
      isAgentMcpAuthWarning({
        kind: "agent_mcp_verification",
        status: "failed",
        failureCode: "connection_failed",
      }),
    ).toBe(false);
    expect(
      isAgentMcpAuthWarning({
        kind: "agent_mcp_verification",
        status: "failed",
        failureCode: "approval_failed",
      }),
    ).toBe(false);
    expect(
      isAgentMcpAuthWarning({
        kind: "agent_mcp_verification",
        status: "failed",
        failureCode: "unavailable",
      }),
    ).toBe(false);
  });

  it("does not classify completed verification steps as auth warnings", () => {
    expect(
      isAgentMcpAuthWarning({
        kind: "agent_mcp_verification",
        status: "completed",
        failureCode: "authentication_required",
      }),
    ).toBe(false);
  });
});
