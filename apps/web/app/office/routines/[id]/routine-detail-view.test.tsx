import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";
import { RoutineDetailView } from "./routine-detail-view";
import { reconcileCronTrigger } from "./cron-reconcile";
import {
  OfficeTopbarChromeProvider,
  useOfficeTopbarChrome,
} from "../../components/office-topbar-context";

const refreshMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ refresh: refreshMock, push: vi.fn(), replace: vi.fn() }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

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

vi.mock("./cron-reconcile", async () => {
  const actual = await vi.importActual<typeof import("./cron-reconcile")>("./cron-reconcile");
  return {
    ...actual,
    reconcileCronTrigger: vi.fn(),
  };
});

import { toast } from "sonner";

const reconcileCronTriggerMock = vi.mocked(reconcileCronTrigger);

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

const relistedCronTrigger: RoutineTrigger = {
  id: "trigger-2",
  routineId: "routine-1",
  kind: "cron",
  cronExpression: "*/10 * * * *",
  timezone: "UTC",
  nextRunAt: "2026-06-06T00:00:00Z",
  enabled: true,
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
};

const NO_TRIGGERS: RoutineTrigger[] = [];

// Save/Run Now are contributed through useOfficeTopbar's actions slot (a
// context the office shell's topbar renders elsewhere), not RoutineDetailView's
// own return tree, so a direct render needs the provider plus a consumer that
// actually displays the slot for the buttons to be reachable at all.
function TopbarActions() {
  const chrome = useOfficeTopbarChrome();
  return <>{chrome?.actions}</>;
}

function renderDetailView(routine: Routine, triggers: RoutineTrigger[]) {
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: { ...defaultOfficeState.office },
      }}
    >
      <OfficeTopbarChromeProvider>
        <RoutineDetailView initialRoutine={routine} initialTriggers={triggers} />
        <TopbarActions />
      </OfficeTopbarChromeProvider>
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

  it("shows no next-fire countdown for a routine loaded with an unrecognized status", () => {
    renderDetailView({ ...baseRoutine, status: "draft" }, [cronTrigger]);
    expect(screen.getByText("Next fire: -")).toBeTruthy();
    expect(screen.queryByText(/Next fire: 5\/5\/2026/)).toBeNull();
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

// AC-003.17: a stored status outside the three selectable options renders no
// selection (the control does not invent a value) and a save omits the field
// rather than sending an invented one.
describe("RoutineDetailView draft status normalization (AC-003.17)", () => {
  it("shows no selected status option when the stored status is not one of the three known values", () => {
    renderDetailView({ ...baseRoutine, status: "draft" }, NO_TRIGGERS);
    const statusField = screen.getByText("Status").closest("div") as HTMLElement;
    const statusCombobox = within(statusField).getByRole("combobox");
    expect(statusCombobox.textContent).toBe("");
  });

  it("keeps the known status selected when it is one of the three options", () => {
    renderDetailView({ ...baseRoutine, status: "paused" }, NO_TRIGGERS);
    const statusField = screen.getByText("Status").closest("div") as HTMLElement;
    expect(within(statusField).getByText("Paused")).toBeTruthy();
  });

  it("omits status from the save patch when the stored status was unrecognized", async () => {
    const { updateRoutine } = await import("@/lib/api/domains/office-api");
    reconcileCronTriggerMock.mockResolvedValue({ kind: "unchanged" });
    renderDetailView({ ...baseRoutine, status: "draft" }, NO_TRIGGERS);

    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(updateRoutine).toHaveBeenCalledWith(
        "routine-1",
        expect.objectContaining({ status: undefined }),
      );
    });
  });
});

// AC-002.9 / SRF-34: describeCronOutcome (through handleSave) must map every
// CronReconcileOutcome branch to the correct toast kind, message, and
// refresh decision — including the SRF-34 uncovered cell (a failed trigger
// mutation whose re-list also fails), which must show the fate-unknown
// message rather than AC-002.9's "saved but schedule was not" text.
describe("RoutineDetailView save outcome mapping (AC-002.9, SRF-34)", () => {
  function clickSave() {
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
  }

  it("shows the plain saved toast and refreshes when the cron draft is unchanged", async () => {
    reconcileCronTriggerMock.mockResolvedValue({ kind: "unchanged" });
    renderDetailView(baseRoutine, NO_TRIGGERS);

    clickSave();

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Routine saved"));
    expect(toast.error).not.toHaveBeenCalled();
    expect(refreshMock).toHaveBeenCalled();
  });

  it("shows the plain saved toast and refreshes on a successful reconcile with a fresh trigger list", async () => {
    reconcileCronTriggerMock.mockResolvedValue({ kind: "success", triggers: [] });
    renderDetailView(baseRoutine, NO_TRIGGERS);

    clickSave();

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Routine saved"));
    expect(refreshMock).toHaveBeenCalled();
  });

  it("shows the stale-schedule toast and does not refresh when a successful reconcile's re-list fails", async () => {
    reconcileCronTriggerMock.mockResolvedValue({ kind: "success", triggers: null });
    renderDetailView(baseRoutine, NO_TRIGGERS);

    clickSave();

    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith(
        "The routine was saved. The displayed schedule may be stale until you reload the page.",
      ),
    );
    expect(refreshMock).not.toHaveBeenCalled();
  });

  it("shows the fate-unknown toast, not AC-002.9's saved-but-not text, when a failed create's re-list also fails (SRF-34)", async () => {
    reconcileCronTriggerMock.mockResolvedValue({
      kind: "create-failed",
      message: "invalid cron expression",
      triggers: null,
    });
    renderDetailView(baseRoutine, NO_TRIGGERS);

    clickSave();

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "The routine was saved, but the schedule's fate is unknown. Reload the page to see the current state.",
      ),
    );
    expect(toast.success).not.toHaveBeenCalled();
    expect(refreshMock).not.toHaveBeenCalled();
  });

  it("shows the fate-unknown toast when a failed delete's re-list also fails (SRF-34)", async () => {
    reconcileCronTriggerMock.mockResolvedValue({
      kind: "delete-failed",
      message: "server error",
      triggers: null,
    });
    renderDetailView(baseRoutine, NO_TRIGGERS);

    clickSave();

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "The routine was saved, but the schedule's fate is unknown. Reload the page to see the current state.",
      ),
    );
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("names the create failure, does not refresh, and renders the re-listed schedule when the re-list succeeds", async () => {
    reconcileCronTriggerMock.mockResolvedValue({
      kind: "create-failed",
      message: "invalid cron expression",
      triggers: [relistedCronTrigger],
    });
    renderDetailView(baseRoutine, [cronTrigger]);

    clickSave();

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "The routine was saved, but the schedule change failed: invalid cron expression",
      ),
    );
    expect(refreshMock).not.toHaveBeenCalled();
    // Proves setTriggers(outcome.triggers) actually reaches the DOM: the
    // Schedule card must show the re-listed trigger's next fire, not the
    // pre-save trigger it replaced.
    expect(screen.getByText(/Next fire: 6\/6\/2026/)).toBeTruthy();
    expect(screen.queryByText(/Next fire: 5\/5\/2026/)).toBeNull();
  });

  it("names the delete failure, the surviving cron count, and renders the re-listed schedule when the re-list succeeds", async () => {
    reconcileCronTriggerMock.mockResolvedValue({
      kind: "delete-failed",
      message: "server error",
      triggers: [relistedCronTrigger],
    });
    renderDetailView(baseRoutine, [cronTrigger]);

    clickSave();

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "The routine was saved, but removing the old schedule failed: server error. 1 cron schedule is now active.",
      ),
    );
    expect(refreshMock).not.toHaveBeenCalled();
    expect(screen.getByText(/Next fire: 6\/6\/2026/)).toBeTruthy();
    expect(screen.queryByText(/Next fire: 5\/5\/2026/)).toBeNull();
  });
});
