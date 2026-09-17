import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { RoutinesContent } from "./routines-content";

const listRoutinesMock = vi.fn();
const listAllRoutineRunsMock = vi.fn();
const listRoutineTriggersMock = vi.fn();
const createRoutineMock = vi.fn();
const createRoutineTriggerMock = vi.fn();

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    listRoutines: (...args: [string]) => listRoutinesMock(...args),
    listAllRoutineRuns: (...args: [string]) => listAllRoutineRunsMock(...args),
    listRoutineTriggers: (...args: [string]) => listRoutineTriggersMock(...args),
    createRoutine: (...args: [string, unknown]) => createRoutineMock(...args),
    createRoutineTrigger: (...args: [string, unknown]) => createRoutineTriggerMock(...args),
  };
});

const toastError = vi.fn();
const toastSuccess = vi.fn();
vi.mock("@/lib/toast/sonner", () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const WORKSPACE_ID = "ws-1";

const AGENT = {
  id: "agent-1",
  workspaceId: WORKSPACE_ID,
  name: "Worker",
  role: "worker",
  status: "idle",
  agentProfileId: "profile-1",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  permissions: {},
  pauseReason: "",
  budgetMonthlyCents: 0,
  maxConcurrentSessions: 1,
} as AgentProfile;

function renderContent() {
  listRoutinesMock.mockResolvedValue({ routines: [] });
  listAllRoutineRunsMock.mockResolvedValue({ runs: [] });
  listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: {
          ...defaultOfficeState.office,
          agentProfilesByWorkspaceId: { [WORKSPACE_ID]: [AGENT] },
        },
      }}
    >
      <RoutinesContent />
    </StateProvider>,
  );
}

// Fills the Details step (name + assignee, both required to advance) and the
// Task Template step (no required fields), landing on the Schedule step,
// where the default trigger kind is already "cron" with an empty expression.
function goToScheduleStepAndSetCron(expression: string) {
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Nightly digest" } });
  fireEvent.click(screen.getAllByRole("combobox")[0]);
  fireEvent.click(within(screen.getByRole("listbox")).getByRole("option", { name: "Worker" }));
  fireEvent.click(screen.getByRole("button", { name: /next/i }));
  fireEvent.click(screen.getByRole("button", { name: /next/i }));
  fireEvent.change(screen.getByPlaceholderText("0 9 * * *"), { target: { value: expression } });
}

describe("RoutinesContent create-routine trigger failure (AC-002.8)", () => {
  it("reports the routine as created without a schedule, closes the dialog, and refreshes the list when trigger creation fails", async () => {
    createRoutineMock.mockResolvedValue({ id: "routine-1" });
    createRoutineTriggerMock.mockRejectedValue(
      new Error("cron trigger requires a cron_expression"),
    );
    renderContent();

    fireEvent.click(screen.getByRole("button", { name: /new routine/i }));
    goToScheduleStepAndSetCron("0 9 * * *");
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        "Routine created, but its schedule could not be saved",
      ),
    );
    expect(createRoutineMock).toHaveBeenCalledTimes(1);
    expect(createRoutineTriggerMock).toHaveBeenCalledWith(
      "routine-1",
      expect.objectContaining({ kind: "cron", cronExpression: "0 9 * * *" }),
    );
    // Dialog closed rather than left open for a retry that would create a
    // second routine.
    expect(screen.queryByRole("dialog")).toBeNull();
    // The routine list was refreshed after the routine (not the trigger) was created.
    expect(listRoutinesMock).toHaveBeenCalledTimes(2);
    expect(toastSuccess).not.toHaveBeenCalled();
  });

  it("reports plain success when both the routine and its trigger are created", async () => {
    createRoutineMock.mockResolvedValue({ id: "routine-1" });
    createRoutineTriggerMock.mockResolvedValue({ trigger: null });
    renderContent();

    fireEvent.click(screen.getByRole("button", { name: /new routine/i }));
    goToScheduleStepAndSetCron("0 9 * * *");
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => expect(toastSuccess).toHaveBeenCalledWith("Routine created"));
    expect(toastError).not.toHaveBeenCalled();
  });

  it("still shows the required error toast when the post-failure refresh itself fails", async () => {
    createRoutineMock.mockResolvedValue({ id: "routine-1" });
    createRoutineTriggerMock.mockRejectedValue(
      new Error("cron trigger requires a cron_expression"),
    );
    // First call is the initial mount fetch; the second is the refresh
    // handleCreate's catch block issues after the trigger create fails.
    listRoutinesMock.mockResolvedValueOnce({ routines: [] });
    listRoutinesMock.mockRejectedValueOnce(new Error("network down"));
    renderContent();

    fireEvent.click(screen.getByRole("button", { name: /new routine/i }));
    goToScheduleStepAndSetCron("0 9 * * *");
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        "Routine created, but its schedule could not be saved",
      ),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("reports the generic create failure, not the AC-002.8 message, when the routine itself fails to create", async () => {
    createRoutineMock.mockRejectedValue(new Error("workspace not found"));
    renderContent();

    fireEvent.click(screen.getByRole("button", { name: /new routine/i }));
    goToScheduleStepAndSetCron("0 9 * * *");
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => expect(toastError).toHaveBeenCalledWith("workspace not found"));
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
  });

  it("extracts the routine id from the real backend's wrapped { routine: { id } } create response shape", async () => {
    createRoutineMock.mockResolvedValue({ routine: { id: "routine-1" } });
    createRoutineTriggerMock.mockRejectedValue(
      new Error("cron trigger requires a cron_expression"),
    );
    renderContent();

    fireEvent.click(screen.getByRole("button", { name: /new routine/i }));
    goToScheduleStepAndSetCron("0 9 * * *");
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        "Routine created, but its schedule could not be saved",
      ),
    );
    expect(createRoutineTriggerMock).toHaveBeenCalledWith(
      "routine-1",
      expect.objectContaining({ kind: "cron", cronExpression: "0 9 * * *" }),
    );
  });
});
