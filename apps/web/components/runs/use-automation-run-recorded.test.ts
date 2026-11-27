import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { emitAutomationRunRecorded } from "@/lib/automation-run-recorded";
import {
  AUTOMATION_RUN_RECORDED_DEBOUNCE_MS,
  useAutomationRunRecorded,
} from "./use-automation-run-recorded";

function recorded(automationId: string, workspaceId = "ws-1") {
  emitAutomationRunRecorded({
    workspace_id: workspaceId,
    automation_id: automationId,
    run_id: `${automationId}-run`,
    status: "triggered",
  });
}

describe("useAutomationRunRecorded", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  // @covers AC-OFFICE-AUTOMATION-RUNS-001.23
  it("refreshes once per burst of runs in its workspace", () => {
    const refresh = vi.fn();
    renderHook(() => useAutomationRunRecorded("ws-1", undefined, refresh));
    recorded("auto-1");
    recorded("auto-2");
    recorded("auto-3", "ws-2");
    expect(refresh).not.toHaveBeenCalled();
    vi.advanceTimersByTime(AUTOMATION_RUN_RECORDED_DEBOUNCE_MS);
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  // @covers AC-OFFICE-AUTOMATION-RUNS-001.23
  it("ignores other automations when scoped to one, and stops after unmount", () => {
    const refresh = vi.fn();
    const { unmount } = renderHook(() => useAutomationRunRecorded("ws-1", "auto-1", refresh));
    recorded("auto-2");
    vi.advanceTimersByTime(AUTOMATION_RUN_RECORDED_DEBOUNCE_MS);
    expect(refresh).not.toHaveBeenCalled();
    recorded("auto-1");
    unmount();
    vi.advanceTimersByTime(AUTOMATION_RUN_RECORDED_DEBOUNCE_MS);
    expect(refresh).not.toHaveBeenCalled();
  });
});
