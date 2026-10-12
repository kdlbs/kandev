import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Popover, PopoverTrigger } from "@kandev/ui/popover";
import type { AttentionStall, AttentionTask } from "@/lib/coordinator/attention";
import { StallEvidenceContent } from "./stall-evidence-content";

afterEach(cleanup);

function stall(overrides: Partial<AttentionStall> = {}): AttentionStall {
  return {
    task_id: "t-1",
    stalled_for_ms: 4 * 3_600_000 + 12 * 60_000,
    last_event_at: "2026-09-27T08:00:00Z",
    detected_at: "2026-09-27T08:01:00Z",
    ...overrides,
  };
}

// Popover content only renders once open; render it directly (not through a
// trigger click) since this suite only asserts the evidence fields.
function renderEvidence(task: AttentionTask, stallRecord: AttentionStall) {
  return render(
    <Popover open>
      <PopoverTrigger />
      <StallEvidenceContent task={task} stall={stallRecord} />
    </Popover>,
  );
}

describe("StallEvidenceContent", () => {
  it("shows stalled for, last event at and detected at", () => {
    renderEvidence({ id: "t-1", title: "Task 1" }, stall());

    expect(screen.getByText("Stalled for")).not.toBeNull();
    expect(screen.getByText("4h 12m")).not.toBeNull();
    expect(screen.getByText("Last event at")).not.toBeNull();
    expect(screen.getByText("Detected at")).not.toBeNull();
  });

  it("shows the agent as running when the primary session is RUNNING or STARTING", () => {
    renderEvidence(
      {
        id: "t-1",
        title: "Task 1",
        statusSummary: { primary_session: { id: "s-1", state: "RUNNING" } },
      },
      stall(),
    );

    expect(screen.getByText("Agent running now")).not.toBeNull();
  });

  it("shows the agent as not running otherwise", () => {
    renderEvidence({ id: "t-1", title: "Task 1" }, stall());

    expect(screen.getByText("Agent not running")).not.toBeNull();
  });
});
