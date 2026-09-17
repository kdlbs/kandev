import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { AgentProfile, Routine, RoutineTrigger } from "@/lib/state/slices/office/types";
import { RoutineRow } from "./routine-row";

afterEach(() => cleanup());

const TIMESTAMP = "2026-01-01T00:00:00Z";
const CRON_EXPRESSION_A = "0 9 * * *";
const CRON_EXPRESSION_B = "0 10 * * *";

const ROUTINE: Routine = {
  id: "routine-1",
  workspaceId: "ws-1",
  name: "Nightly sync",
  taskTemplate: {},
  status: "active",
  concurrencyPolicy: "coalesce_if_active",
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
};

function trigger(overrides: Partial<RoutineTrigger>): RoutineTrigger {
  return {
    id: "trigger-a",
    routineId: "routine-1",
    kind: "cron",
    cronExpression: "*/5 * * * *",
    timezone: "UTC",
    enabled: true,
    createdAt: TIMESTAMP,
    updatedAt: TIMESTAMP,
    ...overrides,
  };
}

function renderRow(routine: Routine, triggers: RoutineTrigger[]) {
  return render(
    <TooltipProvider>
      <RoutineRow
        routine={routine}
        agents={[] as AgentProfile[]}
        triggers={triggers}
        expanded={false}
        onToggle={vi.fn()}
        onRunNow={vi.fn()}
        onDelete={vi.fn()}
        onClick={vi.fn()}
      />
    </TooltipProvider>,
  );
}

describe("RoutineRow trigger display (REQ-003)", () => {
  it("renders the primary cron trigger's expression and countdown together (AC-003.2)", () => {
    const future = new Date(Date.now() + 60 * 60 * 1000).toISOString();
    renderRow(ROUTINE, [trigger({ cronExpression: CRON_EXPRESSION_A, nextRunAt: future })]);
    expect(screen.getByText(CRON_EXPRESSION_A)).toBeTruthy();
    expect(screen.getByText(/next in/i)).toBeTruthy();
  });

  it("renders the countdown from the selector's primary trigger, not the one listed first (AC-003.2)", () => {
    const soon = new Date(Date.now() + 5 * 60 * 1000).toISOString();
    const later = new Date(Date.now() + 6 * 60 * 60 * 1000).toISOString();
    const listedFirstButLaterFire = trigger({
      id: "z",
      cronExpression: CRON_EXPRESSION_B,
      nextRunAt: later,
    });
    const listedSecondButPrimary = trigger({
      id: "a",
      cronExpression: CRON_EXPRESSION_A,
      nextRunAt: soon,
    });
    renderRow(ROUTINE, [listedFirstButLaterFire, listedSecondButPrimary]);
    expect(screen.getByText(CRON_EXPRESSION_A)).toBeTruthy();
    expect(screen.queryByText(CRON_EXPRESSION_B)).toBeNull();
    expect(screen.getByText(/next in/i)).toBeTruthy();
  });

  it("renders the lowest-id cron trigger's expression with no countdown when there is no primary (AC-003.3)", () => {
    const noNextRunAt = trigger({ id: "b", cronExpression: CRON_EXPRESSION_A });
    const disabled = trigger({ id: "a", cronExpression: CRON_EXPRESSION_B, enabled: false });
    renderRow(ROUTINE, [noNextRunAt, disabled]);
    expect(screen.getByText(CRON_EXPRESSION_B)).toBeTruthy();
    expect(screen.queryByText(/next in/i)).toBeNull();
  });

  it("renders neither expression nor countdown when the routine has no cron trigger (AC-003.4)", () => {
    renderRow(ROUTINE, [trigger({ kind: "webhook" })]);
    expect(screen.queryByText(/\*|next in/i)).toBeNull();
  });

  it("suppresses the countdown for a non-firing routine regardless of primary selection (AC-003.6)", () => {
    const future = new Date(Date.now() + 60 * 60 * 1000).toISOString();
    renderRow({ ...ROUTINE, status: "paused" }, [trigger({ nextRunAt: future })]);
    expect(screen.queryByText(/next in/i)).toBeNull();
  });
});
