import { afterEach, describe, expect, it, vi } from "vitest";

import {
  buildFrontendErrorReport,
  deriveTaskID,
  scheduleFrontendErrorReport,
  sendFrontendErrorReport,
} from "./frontend-error-log-api";

describe("frontend error log API", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    window.history.replaceState({}, "", "/");
  });

  it.each([
    ["/t/task-one", "task-one"],
    ["/tasks/task-two", "task-two"],
    ["/office/tasks/task-three", "task-three"],
    ["/settings?taskId=query-task", "query-task"],
    ["/office/projects/task-four", undefined],
  ])("derives task IDs only from recognized routes: %s", (route, expected) => {
    expect(deriveTaskID(new URL(route, "http://localhost"))).toBe(expected);
  });

  it("builds bounded browser context without serializing arbitrary React nodes", () => {
    window.history.replaceState({}, "", "/t/task-one?token=secret#private");
    const report = buildFrontendErrorReport({
      source: "sonner",
      title: "Failed to save",
      description: { type: "div", props: { secret: "must-not-be-walked" } },
      error: new TypeError("Failed to fetch"),
    });

    expect(report.source).toBe("sonner");
    expect(report.title).toBe("Failed to save");
    expect(report.description).toBe("[object]");
    expect(report.task_id).toBe("task-one");
    expect(report.error).toMatchObject({ name: "TypeError", message: "Failed to fetch" });
    expect(report.url).toBe(`${window.location.origin}/t/task-one`);
    expect(report.url).not.toContain("secret");
    expect(report.viewport).toEqual({ width: window.innerWidth, height: window.innerHeight });
    expect(JSON.stringify(report)).not.toContain("must-not-be-walked");
  });

  it("uses an authenticated two-second request and silently accepts 204", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(null, { status: 204 }));
    await sendFrontendErrorReport({
      source: "sonner",
      title: "Failed",
      client_timestamp: new Date().toISOString(),
    });

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/system/logs/frontend-errors"),
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        signal: expect.any(AbortSignal),
      }),
    );
  });

  it("schedules collection off the toast call stack and swallows failures", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("offline"));
    scheduleFrontendErrorReport({ source: "toast-provider", title: "Failed" });
    expect(fetchMock).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());
  });
});
