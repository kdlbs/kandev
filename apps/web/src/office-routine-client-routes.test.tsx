import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";
import { RoutineDetailRoute } from "./office-routine-client-routes";

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn(), replace: vi.fn() }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { getRoutine, listRoutineTriggers } = vi.hoisted(() => ({
  getRoutine: vi.fn(),
  listRoutineTriggers: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return { ...actual, getRoutine, listRoutineTriggers };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const WORKSPACE_ID = "ws-1";
const TIMESTAMP = "2026-05-04T00:00:00Z";
const ROUTINE_ID = "routine-1";

const routine: Routine = {
  id: ROUTINE_ID,
  workspaceId: WORKSPACE_ID,
  name: "Nightly sync",
  taskTemplate: {},
  status: "active",
  concurrencyPolicy: "coalesce_if_active",
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
};

const armedCronTrigger: RoutineTrigger = {
  id: "trigger-1",
  routineId: ROUTINE_ID,
  kind: "cron",
  cronExpression: "*/5 * * * *",
  timezone: "UTC",
  nextRunAt: "2026-05-05T00:00:00Z",
  enabled: true,
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
};

function renderRoute() {
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: { ...defaultOfficeState.office },
      }}
    >
      <TooltipProvider>
        <RoutineDetailRoute routineId={ROUTINE_ID} />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("RoutineDetailRoute — trigger-list load failure", () => {
  it("does not silently render as if the routine had no trigger when the trigger list fails to load", async () => {
    getRoutine.mockResolvedValue(routine);
    listRoutineTriggers.mockRejectedValue(new Error("network down"));

    renderRoute();

    await waitFor(() => expect(screen.getByText("network down")).toBeTruthy());
    // The routine's own detail form (and its trigger-reconciliation state,
    // which would otherwise treat the already-armed cron trigger as absent
    // and let a later Save create a duplicate) must never mount on this path.
    expect(screen.queryByDisplayValue(routine.name)).toBeNull();
  });

  it("still renders the routine's real trigger when both loads succeed", async () => {
    getRoutine.mockResolvedValue(routine);
    listRoutineTriggers.mockResolvedValue({ triggers: [armedCronTrigger] });

    renderRoute();

    await waitFor(() => expect(screen.getByDisplayValue(routine.name)).toBeTruthy());
    expect(screen.getByDisplayValue(armedCronTrigger.cronExpression ?? "")).toBeTruthy();
  });
});
