import { describe, expect, it } from "vitest";
import type { Task } from "@/lib/types/http";
import { buildDebugEntries, resolveTaskProps } from "./task-page-content-helpers";

function baseParams(overrides: Partial<Parameters<typeof buildDebugEntries>[0]> = {}) {
  return {
    connectionStatus: "connected",
    task: null,
    effectiveSessionId: "s1",
    taskSessionState: "RUNNING",
    isAgentWorking: true,
    resumptionState: "idle",
    resumptionError: null,
    agentctlStatus: { status: "ready", isReady: true },
    previewOpen: false,
    previewStage: "closed",
    previewUrl: "",
    devProcessId: undefined,
    devProcessStatus: null,
    ...overrides,
  };
}

describe("buildDebugEntries", () => {
  it("includes active session ACP metadata", () => {
    const entries = buildDebugEntries(
      baseParams({
        activeSessionMetadata: {
          acp: {
            session_id: "acp-1",
            title: "List files",
            updated_at: "2026-06-13T19:37:46Z",
            meta: { cursor: { requestId: "req-1" } },
          },
        },
      }),
    );

    expect(entries.acp_session_id).toBe("acp-1");
    expect(entries.acp_session_title).toBe("List files");
    expect(entries.acp_session_updated_at).toBe("2026-06-13T19:37:46Z");
    expect(entries.acp_meta).toEqual({ cursor: { requestId: "req-1" } });
  });
});

describe("resolveTaskProps", () => {
  it("exposes linked GitHub issue metadata for the top bar", () => {
    const props = resolveTaskProps(
      {
        id: "task-1",
        title: "Link issue",
        metadata: {
          issue_url: "https://github.com/kdlbs/kandev/issues/1470",
          issue_number: 1470,
        },
      } as unknown as Task,
      null,
    );

    expect(props.issueUrl).toBe("https://github.com/kdlbs/kandev/issues/1470");
    expect(props.issueNumber).toBe(1470);
  });
});
