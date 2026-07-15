import { describe, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";
import {
  buildDebugEntries,
  hasResolvedTaskDetails,
  resolveTaskContentState,
  resolveTaskProps,
  syncActiveTaskSession,
} from "./task-page-content-helpers";

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

describe("resolveTaskContentState", () => {
  it("keeps showing the loading state until the component mounts", () => {
    expect(
      resolveTaskContentState({
        isMounted: false,
        hasTask: false,
        hasTaskLoadError: true,
      }),
    ).toBe("loading");
  });

  it("surfaces task load failures after mount", () => {
    expect(
      resolveTaskContentState({
        isMounted: true,
        hasTask: false,
        hasTaskLoadError: true,
      }),
    ).toBe("error");
  });

  it("surfaces task load failures even when a placeholder task exists", () => {
    expect(
      resolveTaskContentState({
        isMounted: true,
        hasTask: true,
        hasTaskLoadError: true,
      }),
    ).toBe("error");
  });

  it("treats a resolved task as ready", () => {
    expect(
      resolveTaskContentState({
        isMounted: true,
        hasTask: true,
        hasTaskLoadError: false,
      }),
    ).toBe("ready");
  });
});

describe("hasResolvedTaskDetails", () => {
  it("returns true when fetched details match the effective task", () => {
    expect(
      hasResolvedTaskDetails({
        effectiveTaskId: "task-1",
        taskDetailsId: "task-1",
        initialTaskId: null,
      }),
    ).toBe(true);
  });

  it("returns true when SSR task details match the effective task", () => {
    expect(
      hasResolvedTaskDetails({
        effectiveTaskId: "task-1",
        taskDetailsId: null,
        initialTaskId: "task-1",
      }),
    ).toBe(true);
  });

  it("returns false for kanban-only placeholder tasks", () => {
    expect(
      hasResolvedTaskDetails({
        effectiveTaskId: "task-1",
        taskDetailsId: "task-2",
        initialTaskId: null,
      }),
    ).toBe(false);
  });

  it("returns false when there is no effective task", () => {
    expect(
      hasResolvedTaskDetails({
        effectiveTaskId: null,
        taskDetailsId: "task-1",
        initialTaskId: "task-1",
      }),
    ).toBe(false);
  });
});

describe("syncActiveTaskSession", () => {
  it("restores the initial session without creating a user pin", () => {
    const setActiveSessionAuto = vi.fn();

    syncActiveTaskSession({
      initialTaskId: "task-1",
      fallbackTaskId: null,
      initialSessionId: "session-1",
      setActiveSessionAuto,
      setActiveTask: vi.fn(),
    });

    expect(setActiveSessionAuto).toHaveBeenCalledWith("task-1", "session-1");
  });
});
