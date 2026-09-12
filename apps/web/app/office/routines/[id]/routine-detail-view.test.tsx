import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";
import { RoutineDetailView } from "./routine-detail-view";

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn(), replace: vi.fn() }),
}));

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    updateRoutine: vi.fn().mockResolvedValue({}),
    runRoutine: vi.fn().mockResolvedValue({}),
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const WORKSPACE_ID = "ws-1";
const TIMESTAMP = "2026-05-04T00:00:00Z";

const baseRoutine: Routine = {
  id: "routine-1",
  workspaceId: WORKSPACE_ID,
  name: "Nightly sync",
  taskTemplate: {},
  status: "active",
  concurrencyPolicy: "coalesce_if_active",
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
};

const cronTrigger: RoutineTrigger = {
  id: "trigger-1",
  routineId: "routine-1",
  kind: "cron",
  cronExpression: "*/5 * * * *",
  timezone: "UTC",
  nextRunAt: "2026-05-05T00:00:00Z",
  enabled: true,
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
};

function renderDetailView(routine: Routine, triggers: RoutineTrigger[]) {
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: { ...defaultOfficeState.office },
      }}
    >
      <RoutineDetailView initialRoutine={routine} initialTriggers={triggers} />
    </StateProvider>,
  );
}

describe("RoutineDetailView next-fire display", () => {
  it("hides the next-fire countdown as soon as status is changed to paused, before saving", async () => {
    renderDetailView(baseRoutine, [cronTrigger]);

    expect(screen.getByText(/Next fire: 5\/5\/2026/)).toBeTruthy();

    const statusField = screen.getByText("Status").closest("div") as HTMLElement;
    fireEvent.click(within(statusField).getByRole("combobox"));
    const listbox = await screen.findByRole("listbox");
    fireEvent.click(within(listbox).getByRole("option", { name: "Paused" }));

    expect(screen.getByText("Next fire: -")).toBeTruthy();
    expect(screen.queryByText(/Next fire: 5\/5\/2026/)).toBeNull();
  });

  it("keeps showing the next-fire countdown for a routine loaded as active", () => {
    renderDetailView(baseRoutine, [cronTrigger]);
    expect(screen.getByText(/Next fire: 5\/5\/2026/)).toBeTruthy();
  });

  it("shows no next-fire countdown for a routine loaded as paused", () => {
    renderDetailView({ ...baseRoutine, status: "paused" }, [cronTrigger]);
    expect(screen.getByText("Next fire: -")).toBeTruthy();
  });
});
