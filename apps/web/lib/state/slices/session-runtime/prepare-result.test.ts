import { describe, it, expect } from "vitest";
import { prepareResultToSessionState } from "./prepare-result";

describe("prepareResultToSessionState", () => {
  it("returns null when metadata is missing or has no prepare_result", () => {
    expect(prepareResultToSessionState("s1", null)).toBeNull();
    expect(prepareResultToSessionState("s1", undefined)).toBeNull();
    expect(prepareResultToSessionState("s1", {})).toBeNull();
    expect(prepareResultToSessionState("s1", { other: 1 })).toBeNull();
  });

  it("maps snake_case prepare_result into camelCase SessionPrepareState", () => {
    const result = prepareResultToSessionState("s1", {
      prepare_result: {
        status: "completed",
        error_message: "boom",
        duration_ms: 1234,
        preparation_id: "attempt-1",
        preparation_started_at: "2026-09-28T18:00:00.123456789Z",
        steps: [
          {
            name: "clone",
            kind: "remote_helper_download",
            remote_platform: "linux/amd64",
            mcp_server_id: "server-a",
            mcp_provider: "cursor",
            failure_code: "timeout",
            command: "git clone",
            status: "ok",
            output: "done",
            error: "err",
            warning: "warn",
            warning_detail: "detail",
            started_at: "2026-01-01T00:00:00Z",
            ended_at: "2026-01-01T00:00:05Z",
          },
        ],
      },
    });

    expect(result).toEqual({
      sessionId: "s1",
      status: "completed",
      errorMessage: "boom",
      durationMs: 1234,
      preparationId: "attempt-1",
      preparationStartedAt: "2026-09-28T18:00:00.123456789Z",
      steps: [
        {
          name: "clone",
          kind: "remote_helper_download",
          remotePlatform: "linux/amd64",
          mcpServerId: "server-a",
          mcpProvider: "cursor",
          failureCode: "timeout",
          command: "git clone",
          status: "ok",
          output: "done",
          error: "err",
          warning: "warn",
          warningDetail: "detail",
          startedAt: "2026-01-01T00:00:00Z",
          endedAt: "2026-01-01T00:00:05Z",
        },
      ],
    });
  });

  it("defaults status to completed and steps to empty when absent", () => {
    const result = prepareResultToSessionState("s1", { prepare_result: {} });
    expect(result).toEqual({
      sessionId: "s1",
      status: "completed",
      errorMessage: undefined,
      durationMs: undefined,
      steps: [],
    });
  });
});

describe("prepareResultToSessionState MCP diagnostics", () => {
  it("hydrates bounded MCP diagnostics and continues to suppress legacy raw MCP fields", () => {
    const diagnostic = {
      operation: "enable",
      stage: "wait",
      kind: "output_wait_timeout",
      message: "exec: WaitDelay expired before I/O complete",
      exit_code: 0,
    };
    const result = prepareResultToSessionState("s1", {
      prepare_result: {
        steps: [
          {
            name: "raw name",
            kind: "agent_mcp_approval",
            mcp_server_id: "server-a",
            status: "failed",
            command: "raw command",
            output: "raw stdout",
            error: "raw stderr",
            mcp_diagnostic: diagnostic,
          },
          {
            name: "raw name",
            kind: "agent_mcp_verification",
            mcp_server_id: "server-b",
            status: "failed",
            mcp_diagnostic: { ...diagnostic, stage: "unknown" },
          },
        ],
      },
    });

    expect(result?.steps).toMatchObject([
      {
        name: "",
        kind: "agent_mcp_approval",
        mcpServerId: "server-a",
        status: "failed",
        mcpDiagnostic: {
          operation: "enable",
          stage: "wait",
          kind: "output_wait_timeout",
          message: "exec: WaitDelay expired before I/O complete",
          exitCode: 0,
        },
      },
      {
        name: "",
        kind: "agent_mcp_verification",
        mcpServerId: "server-b",
        status: "failed",
      },
    ]);
  });
});
