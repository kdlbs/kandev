import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { Routine } from "@/lib/state/slices/office/types";
import { CoordinatorRoutineHint } from "./layout";

afterEach(() => {
  cleanup();
});

const WORKSPACE_ID = "ws-1";
const AGENT_ID = "agent-1";
const TIMESTAMP = "2026-05-04T00:00:00Z";
const HINT_LABEL = "No scheduled wake-ups; manage routines";

function baseRoutine(overrides: Partial<Routine> = {}): Routine {
  return {
    id: "routine-1",
    workspaceId: WORKSPACE_ID,
    name: "Daily standup",
    taskTemplate: {},
    status: "active",
    concurrencyPolicy: "coalesce_if_active",
    assigneeAgentProfileId: AGENT_ID,
    createdAt: TIMESTAMP,
    updatedAt: TIMESTAMP,
    ...overrides,
  };
}

function renderHint(routines: Routine[], agentRole = "ceo") {
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: { ...defaultOfficeState.office, routines },
      }}
    >
      <TooltipProvider>
        <CoordinatorRoutineHint agentId={AGENT_ID} agentRole={agentRole} />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("CoordinatorRoutineHint", () => {
  it("shows no hint for a coordinator with a routine whose status is active", () => {
    renderHint([baseRoutine({ status: "active" })]);
    expect(screen.queryByLabelText(HINT_LABEL)).toBeNull();
  });

  it("shows no hint for a coordinator with a routine whose status is the empty string (no writer set one)", () => {
    renderHint([baseRoutine({ status: "" })]);
    expect(screen.queryByLabelText(HINT_LABEL)).toBeNull();
  });

  it("shows the hint for a coordinator with a paused routine", () => {
    renderHint([baseRoutine({ status: "paused" })]);
    expect(screen.getByLabelText(HINT_LABEL)).toBeTruthy();
  });

  it("shows the hint for a coordinator with no routines at all", () => {
    renderHint([]);
    expect(screen.getByLabelText(HINT_LABEL)).toBeTruthy();
  });

  it("shows the hint when the only routine targeting this agent is archived", () => {
    renderHint([baseRoutine({ status: "archived" })]);
    expect(screen.getByLabelText(HINT_LABEL)).toBeTruthy();
  });

  it("shows no hint when a routine is active but a different one targeting the agent is paused", () => {
    renderHint([
      baseRoutine({ id: "routine-1", status: "paused" }),
      baseRoutine({ id: "routine-2", status: "active" }),
    ]);
    expect(screen.queryByLabelText(HINT_LABEL)).toBeNull();
  });

  it("ignores an active routine assigned to a different agent", () => {
    renderHint([baseRoutine({ assigneeAgentProfileId: "agent-2", status: "active" })]);
    expect(screen.getByLabelText(HINT_LABEL)).toBeTruthy();
  });

  it("renders nothing for a non-coordinator role, regardless of routines", () => {
    renderHint([], "worker");
    expect(screen.queryByLabelText(HINT_LABEL)).toBeNull();
  });
});
