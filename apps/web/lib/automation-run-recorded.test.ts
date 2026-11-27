import { afterEach, describe, expect, it, vi } from "vitest";
import {
  emitAutomationRunRecorded,
  subscribeAutomationRunRecorded,
} from "./automation-run-recorded";

const RUN = { workspace_id: "ws-1", automation_id: "auto-1", run_id: "run-1", status: "triggered" };

describe("automation run-recorded channel", () => {
  const unsubscribers: Array<() => void> = [];
  afterEach(() => unsubscribers.splice(0).forEach((stop) => stop()));

  // @covers AC-OFFICE-AUTOMATION-RUNS-001.23
  it("delivers each run to current subscribers until they unsubscribe", () => {
    const listener = vi.fn();
    const stop = subscribeAutomationRunRecorded(listener);
    emitAutomationRunRecorded(RUN);
    stop();
    emitAutomationRunRecorded(RUN);
    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener).toHaveBeenCalledWith(RUN);
  });
});
