import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { Routine, ScheduleState } from "@/lib/state/slices/office/types";
import { ScheduleStateBadge, UnarmedScheduleHint } from "./schedule-state-badge";

afterEach(cleanup);

function renderBadge(ui: React.ReactElement) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

function routine(overrides: Record<string, unknown> = {}): Routine {
  return {
    id: "routine-1",
    workspaceId: "ws-1",
    name: "Nightly digest",
    taskTemplate: {},
    status: "active",
    concurrencyPolicy: "skip_if_active",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  } as unknown as Routine;
}

// AC-OFFICE-ROUTINE-ARMING-002.10: every group renders a distinct label, and
// two states from different groups never render the same text.
describe("ScheduleStateBadge", () => {
  const cases: Array<[ScheduleState, string]> = [
    ["armed", "Armed"],
    ["trigger_invalid", "Schedule broken"],
    ["trigger_unscheduled", "Schedule broken"],
    ["trigger_disabled", "Schedule broken"],
    ["event_only", "Event-triggered"],
    ["unscheduled_manual_only", "No schedule"],
    ["unscheduled_no_trigger", "No schedule"],
    ["unknown", "Schedule unknown"],
  ];

  it.each(cases)("renders the group label for schedule state %s", (state, label) => {
    renderBadge(<ScheduleStateBadge routine={routine({ scheduleState: state })} />);
    expect(screen.getByText(label)).toBeTruthy();
  });

  it("never renders event_only as having no schedule (AC-002.11)", () => {
    renderBadge(<ScheduleStateBadge routine={routine({ scheduleState: "event_only" })} />);
    expect(screen.queryByText("No schedule")).toBeNull();
  });

  it("collapses every state in the same group onto the same label", () => {
    const labels = new Set<string>();
    for (const [state, label] of cases) {
      cleanup();
      renderBadge(<ScheduleStateBadge routine={routine({ scheduleState: state })} />);
      labels.add(label);
    }
    // Five distinct groups, not nine distinct per-state labels.
    expect(labels.size).toBe(5);
  });
});

describe("UnarmedScheduleHint", () => {
  it("renders nothing when the unarmed cron trigger list is empty (AC-002.4)", () => {
    const { container } = render(
      <UnarmedScheduleHint routine={routine({ unarmedCronTriggers: [] })} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("distinguishes an unarmed entry that only needs re-arming", () => {
    render(
      <UnarmedScheduleHint
        routine={routine({
          unarmedCronTriggers: [{ triggerId: "t1", reasons: ["disabled"] }],
        })}
      />,
    );
    expect(screen.getByText("Needs re-arming")).toBeTruthy();
  });

  it("distinguishes an unarmed entry that needs its expression or timezone edited", () => {
    render(
      <UnarmedScheduleHint
        routine={routine({
          unarmedCronTriggers: [{ triggerId: "t1", reasons: ["not_schedulable"] }],
        })}
      />,
    );
    expect(screen.getByText("Needs schedule fix")).toBeTruthy();
  });

  it("reports 're-arming' once any entry in a mixed list is still schedulable", () => {
    render(
      <UnarmedScheduleHint
        routine={routine({
          unarmedCronTriggers: [
            { triggerId: "t1", reasons: ["not_schedulable"] },
            { triggerId: "t2", reasons: ["disabled"] },
          ],
        })}
      />,
    );
    expect(screen.getByText("Needs re-arming")).toBeTruthy();
  });
});
