import { afterEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { subscribeAutomationRunRecorded } from "@/lib/automation-run-recorded";
import { registerAutomationRunHandlers } from "./automation-runs";

function storeWithActiveWorkspace(activeId: string): StoreApi<AppState> {
  return { getState: () => ({ workspaces: { activeId } }) } as unknown as StoreApi<AppState>;
}

function message(workspaceId: string) {
  const payload = {
    workspace_id: workspaceId,
    automation_id: "auto-1",
    run_id: "run-1",
    status: "skipped",
  };
  return { type: "notification", action: "automation.run.created", payload } as never;
}

describe("automation run handlers", () => {
  let stop: (() => void) | undefined;
  afterEach(() => stop?.());

  // @covers AC-OFFICE-AUTOMATION-RUNS-001.23
  it("announces runs recorded in the active workspace only", () => {
    const listener = vi.fn();
    stop = subscribeAutomationRunRecorded(listener);
    const handlers = registerAutomationRunHandlers(storeWithActiveWorkspace("ws-1"));
    handlers["automation.run.created"]!(message("ws-2"));
    expect(listener).not.toHaveBeenCalled();
    handlers["automation.run.created"]!(message("ws-1"));
    expect(listener).toHaveBeenCalledWith(expect.objectContaining({ run_id: "run-1" }));
  });
});
