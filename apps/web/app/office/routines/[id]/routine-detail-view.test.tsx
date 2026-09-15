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

const NO_TRIGGERS: RoutineTrigger[] = [];

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

// AC-OFFICE-ROUTINE-CATCHUP-003.8: the catch_up_max control renders under
// the summarizing policy and is absent under skip_missed, in the detail
// view exactly as in the create dialog. Regression coverage for the
// enqueue_missed_with_cap -> summarize_missed rename, which silently
// dropped this control when only the option values/defaults were updated
// without also updating the two "===" guards that decide whether the
// input renders at all.
describe("RoutineDetailView catch-up max control (AC-003.8)", () => {
  it("renders the catch-up max input when the routine is summarize_missed", () => {
    renderDetailView(
      { ...baseRoutine, catchUpPolicy: "summarize_missed", catchUpMax: 25 },
      NO_TRIGGERS,
    );
    expect(screen.getByText(/catch-up max/i)).toBeTruthy();
    expect(screen.getByRole("spinbutton")).toBeTruthy();
  });

  it("does not render the catch-up max input when the routine is skip_missed", () => {
    renderDetailView({ ...baseRoutine, catchUpPolicy: "skip_missed" }, NO_TRIGGERS);
    expect(screen.queryByText(/catch-up max/i)).toBeNull();
    expect(screen.queryByRole("spinbutton")).toBeNull();
  });

  it("falls back to summarize_missed (not the retired value) when catchUpPolicy is unset", () => {
    const { catchUpPolicy: _omit, ...withoutPolicy } = baseRoutine as Routine & {
      catchUpPolicy?: string;
    };
    renderDetailView(withoutPolicy as Routine, NO_TRIGGERS);
    expect(screen.getByText(/catch-up max/i)).toBeTruthy();
    expect(screen.getByRole("spinbutton")).toBeTruthy();
  });
});

// AC-OFFICE-ROUTINE-CATCHUP-003.6: the summarizing policy's own label must
// state a single summarized wake, matching the create dialog.
describe("RoutineDetailView catch-up policy labeling (AC-003.6)", () => {
  it("labels the summarizing policy option as a single wake", () => {
    renderDetailView(
      { ...baseRoutine, catchUpPolicy: "summarize_missed", catchUpMax: 25 },
      NO_TRIGGERS,
    );
    // Comboboxes in DOM order: status, assignee, concurrency policy,
    // catch-up policy, then the trigger card's kind select.
    const catchUpPolicyCombobox = screen.getAllByRole("combobox")[3];
    fireEvent.click(catchUpPolicyCombobox);
    const option = within(screen.getByRole("listbox")).getByRole("option", {
      name: /summarize missed/i,
    });
    expect(option.textContent).toMatch(/once|single/i);
  });
});
