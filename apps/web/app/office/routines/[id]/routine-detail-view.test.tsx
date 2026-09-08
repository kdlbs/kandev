import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    refresh: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    prefetch: vi.fn(),
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: "ws-1" },
      office: { agentProfilesByWorkspaceId: { "ws-1": [] } },
    }),
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  updateRoutine: vi.fn(),
  runRoutine: vi.fn(),
  createRoutineTrigger: vi.fn(),
  deleteRoutineTrigger: vi.fn(),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { RoutineDetailView } from "./routine-detail-view";

afterEach(() => cleanup());

const BASE_ROUTINE: Routine = {
  id: "routine-1",
  workspaceId: "ws-1",
  name: "Nightly digest",
  taskTemplate: {},
  status: "active",
  concurrencyPolicy: "coalesce_if_active",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

const NO_TRIGGERS: RoutineTrigger[] = [];

// AC-OFFICE-ROUTINE-CATCHUP-003.8: the catch_up_max control renders under
// the summarizing policy and is absent under skip_missed, in the detail
// view exactly as in the create dialog. Regression coverage for the
// enqueue_missed_with_cap -> summarize_missed rename, which silently
// dropped this control when only the option values/defaults were updated
// without also updating the two "===" guards that decide whether the
// input renders at all.
describe("RoutineDetailView catch-up max control (AC-003.8)", () => {
  it("renders the catch-up max input when the routine is summarize_missed", () => {
    render(
      <RoutineDetailView
        initialRoutine={{ ...BASE_ROUTINE, catchUpPolicy: "summarize_missed", catchUpMax: 25 }}
        initialTriggers={NO_TRIGGERS}
      />,
    );
    expect(screen.getByText(/catch-up max/i)).toBeTruthy();
    expect(screen.getByRole("spinbutton")).toBeTruthy();
  });

  it("does not render the catch-up max input when the routine is skip_missed", () => {
    render(
      <RoutineDetailView
        initialRoutine={{ ...BASE_ROUTINE, catchUpPolicy: "skip_missed" }}
        initialTriggers={NO_TRIGGERS}
      />,
    );
    expect(screen.queryByText(/catch-up max/i)).toBeNull();
    expect(screen.queryByRole("spinbutton")).toBeNull();
  });

  it("falls back to summarize_missed (not the retired value) when catchUpPolicy is unset", () => {
    const { catchUpPolicy: _omit, ...withoutPolicy } = BASE_ROUTINE;
    render(<RoutineDetailView initialRoutine={withoutPolicy} initialTriggers={NO_TRIGGERS} />);
    expect(screen.getByText(/catch-up max/i)).toBeTruthy();
    expect(screen.getByRole("spinbutton")).toBeTruthy();
  });
});
