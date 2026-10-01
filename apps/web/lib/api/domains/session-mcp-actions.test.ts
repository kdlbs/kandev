import { beforeEach, describe, expect, it, vi } from "vitest";

const { fetchJson } = vi.hoisted(() => ({ fetchJson: vi.fn() }));
const SERVER_ID = "plugin:figma:server";
vi.mock("../client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../client")>();
  return { ...actual, fetchJson };
});
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: vi.fn() }));

import { ApiError } from "../client";
import {
  authenticateAgentMcp,
  isAgentMcpRecoveryBusyError,
  retryAgentMcpConnection,
} from "./session-api";

describe("task MCP recovery API", () => {
  beforeEach(() => fetchJson.mockReset());

  it("posts only the native server ID for authentication", async () => {
    fetchJson.mockResolvedValue({
      terminal_id: "terminal-1",
      task_environment_id: "environment-1",
      label: "Sign in to Figma",
      reused: false,
    });

    await authenticateAgentMcp("session/1", SERVER_ID);

    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session%2F1/mcp/authenticate",
      expect.objectContaining({
        init: {
          method: "POST",
          body: JSON.stringify({ server_id: SERVER_ID }),
        },
      }),
    );
    expect(JSON.stringify(fetchJson.mock.calls[0])).not.toContain("command");
  });

  it("posts only the native server ID to retry and returns typed readiness", async () => {
    fetchJson.mockResolvedValue({
      provider_id: "cursor",
      server_id: SERVER_ID,
      status: "ready",
      tool_count: 3,
    });

    await expect(retryAgentMcpConnection("session-1", SERVER_ID)).resolves.toMatchObject({
      status: "ready",
      tool_count: 3,
    });
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session-1/mcp/retry",
      expect.objectContaining({
        init: {
          method: "POST",
          body: JSON.stringify({ server_id: SERVER_ID }),
        },
      }),
    );
  });

  it("recognizes only the session-busy recovery conflict", () => {
    expect(
      isAgentMcpRecoveryBusyError(
        new ApiError("session is busy", 409, { error_code: "mcp_recovery_session_busy" }),
      ),
    ).toBe(true);
    expect(
      isAgentMcpRecoveryBusyError(new ApiError("conflict", 409, { error_code: "other_conflict" })),
    ).toBe(false);
  });
});
