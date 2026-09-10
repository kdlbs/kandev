import { describe, expect, it } from "vitest";
import type { WorkflowSnapshot } from "@/lib/types/http";
import { mapSnapshotToKanban } from "./session-task-switcher-sheet-helpers";

describe("mapSnapshotToKanban", () => {
  it("preserves the signal-gated flag when switching workflows", () => {
    const snapshot = {
      steps: [
        {
          id: "step-1",
          name: "Review",
          position: 1,
          color: "bg-blue-500",
          auto_advance_requires_signal: true,
        },
      ],
      tasks: [],
    } as unknown as WorkflowSnapshot;

    const state = mapSnapshotToKanban(snapshot, "workflow-1");

    expect(state.steps[0]).toMatchObject({
      auto_advance_requires_signal: true,
    });
  });
});
