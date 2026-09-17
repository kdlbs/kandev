import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { RoutinesContent } from "./routines-content";
import {
  createRoutine,
  createRoutineTrigger,
  listAllRoutineRuns,
  listRoutines,
  listRoutineTriggers,
} from "@/lib/api/domains/office-api";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    listRoutines: vi.fn(),
    listAllRoutineRuns: vi.fn(),
    listRoutineTriggers: vi.fn(),
    createRoutine: vi.fn(),
    createRoutineTrigger: vi.fn(),
  };
});

import { toast } from "sonner";

const listRoutinesMock = vi.mocked(listRoutines);
const listAllRoutineRunsMock = vi.mocked(listAllRoutineRuns);
const listRoutineTriggersMock = vi.mocked(listRoutineTriggers);
const createRoutineMock = vi.mocked(createRoutine);
const createRoutineTriggerMock = vi.mocked(createRoutineTrigger);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const WORKSPACE_ID = "ws-1";
const TIMESTAMP = "2026-05-04T00:00:00Z";
const ROUTINE_NAME = "Nightly digest";
const CRON_EXPRESSION_LABEL = "Cron Expression";

const AGENT: AgentProfile = {
  id: "agent-1",
  workspaceId: WORKSPACE_ID,
  name: "Worker",
  role: "worker",
  status: "idle",
  agentProfileId: "profile-1",
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
  permissions: {},
  pauseReason: "",
  budgetMonthlyCents: 0,
  maxConcurrentSessions: 1,
} as AgentProfile;

function renderContent() {
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: {
          ...defaultOfficeState.office,
          agentProfilesByWorkspaceId: { [WORKSPACE_ID]: [AGENT] },
          routines: [],
        },
      }}
    >
      <RoutinesContent />
    </StateProvider>,
  );
}

async function openCreateDialogToScheduleStep() {
  fireEvent.click(screen.getByRole("button", { name: /new routine/i }));
  fireEvent.change(await screen.findByLabelText("Name"), {
    target: { value: ROUTINE_NAME },
  });
  fireEvent.click(screen.getAllByRole("combobox")[0]);
  fireEvent.click(within(screen.getByRole("listbox")).getByRole("option", { name: "Worker" }));
  fireEvent.click(screen.getByRole("button", { name: /next/i }));
  fireEvent.click(screen.getByRole("button", { name: /next/i }));
}

describe("RoutinesContent create-routine cron arm (AC-002.2, AC-002.10)", () => {
  it("arms a cron trigger after the routine is created and shows one success toast", async () => {
    listRoutinesMock.mockResolvedValue({ routines: [] });
    listAllRoutineRunsMock.mockResolvedValue({ runs: [] });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
    createRoutineMock.mockResolvedValue({
      id: "routine-1",
      workspaceId: WORKSPACE_ID,
      name: ROUTINE_NAME,
      taskTemplate: {},
      status: "active",
      concurrencyPolicy: "coalesce_if_active",
      createdAt: TIMESTAMP,
      updatedAt: TIMESTAMP,
    });
    createRoutineTriggerMock.mockResolvedValue({
      id: "trigger-1",
      routineId: "routine-1",
      kind: "cron",
      cronExpression: "0 9 * * *",
      timezone: "UTC",
      enabled: true,
      createdAt: TIMESTAMP,
      updatedAt: TIMESTAMP,
    });

    renderContent();
    await openCreateDialogToScheduleStep();
    fireEvent.change(screen.getByLabelText(CRON_EXPRESSION_LABEL), {
      target: { value: "0 9 * * *" },
    });
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => {
      expect(createRoutineTriggerMock).toHaveBeenCalledWith("routine-1", {
        kind: "cron",
        cronExpression: "0 9 * * *",
        timezone: "UTC",
      });
    });
    expect(toast.success).toHaveBeenCalledWith("Routine created");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("disables Create for a cron expression that is only whitespace, so no trigger can be armed", async () => {
    renderContent();
    await openCreateDialogToScheduleStep();
    fireEvent.change(screen.getByLabelText(CRON_EXPRESSION_LABEL), {
      target: { value: "   " },
    });

    expect((screen.getByRole("button", { name: /create/i }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("shows an error toast naming the trigger failure and no success toast when the cron trigger create fails", async () => {
    listRoutinesMock.mockResolvedValue({ routines: [] });
    listAllRoutineRunsMock.mockResolvedValue({ runs: [] });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
    createRoutineMock.mockResolvedValue({
      id: "routine-1",
      workspaceId: WORKSPACE_ID,
      name: ROUTINE_NAME,
      taskTemplate: {},
      status: "active",
      concurrencyPolicy: "coalesce_if_active",
      createdAt: TIMESTAMP,
      updatedAt: TIMESTAMP,
    });
    createRoutineTriggerMock.mockRejectedValue(new Error("invalid cron expression"));

    renderContent();
    await openCreateDialogToScheduleStep();
    fireEvent.change(screen.getByLabelText(CRON_EXPRESSION_LABEL), {
      target: { value: "bad cron" },
    });
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        "The routine was created without a schedule: invalid cron expression",
      );
    });
    expect(toast.success).not.toHaveBeenCalled();
    // TS-004: the failure branch must still close the dialog and refetch,
    // not just toast.
    expect(screen.queryByRole("button", { name: /^create$/i })).toBeNull();
    expect(listRoutinesMock).toHaveBeenCalledTimes(2);
  });
});

// Regression coverage: handleCreate's refactor into two try/catch blocks
// (create, then optional cron-arm) left the post-mutation `fetchRoutines()`
// calls unguarded. A refetch failure there must not swallow the toast
// reporting the create/trigger outcome, and must not become an unhandled
// promise rejection.
describe("RoutinesContent post-create refetch failure", () => {
  // The mount effect also calls fetchRoutines, so a fixed call-count
  // assumption ("call 2 is the post-create refetch") is fragile. Instead,
  // resolve every call until the test arms failNextFetch right before the
  // action whose refetch should fail, so the rejection lands on that call
  // regardless of how many mount-time calls preceded it.
  function mockListRoutinesFailingNextCallAfterArmed() {
    let failNext = false;
    listRoutinesMock.mockImplementation(async () => {
      if (failNext) {
        failNext = false;
        throw new Error("network down");
      }
      return { routines: [] };
    });
    return () => {
      failNext = true;
    };
  }

  it("still shows the created toast when the post-success refetch fails", async () => {
    const armFailure = mockListRoutinesFailingNextCallAfterArmed();
    listAllRoutineRunsMock.mockResolvedValue({ runs: [] });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
    createRoutineMock.mockResolvedValue({
      id: "routine-1",
      workspaceId: WORKSPACE_ID,
      name: ROUTINE_NAME,
      taskTemplate: {},
      status: "active",
      concurrencyPolicy: "coalesce_if_active",
      createdAt: TIMESTAMP,
      updatedAt: TIMESTAMP,
    });

    renderContent();
    fireEvent.click(screen.getByRole("button", { name: /new routine/i }));
    fireEvent.change(await screen.findByLabelText("Name"), {
      target: { value: ROUTINE_NAME },
    });
    fireEvent.click(screen.getAllByRole("combobox")[0]);
    fireEvent.click(within(screen.getByRole("listbox")).getByRole("option", { name: "Worker" }));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    // Switch off the default "cron" trigger kind (which disables Create
    // until a cron expression is entered) to exercise the plain
    // no-trigger create path.
    fireEvent.click(screen.getAllByRole("combobox")[0]);
    fireEvent.click(within(screen.getByRole("listbox")).getByRole("option", { name: "Webhook" }));
    armFailure();
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Routine created"));
    expect(toast.error).toHaveBeenCalledWith("Failed to load");
  });

  it("still shows the trigger-failure toast when that branch's refetch also fails", async () => {
    const armFailure = mockListRoutinesFailingNextCallAfterArmed();
    listAllRoutineRunsMock.mockResolvedValue({ runs: [] });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
    createRoutineMock.mockResolvedValue({
      id: "routine-1",
      workspaceId: WORKSPACE_ID,
      name: ROUTINE_NAME,
      taskTemplate: {},
      status: "active",
      concurrencyPolicy: "coalesce_if_active",
      createdAt: TIMESTAMP,
      updatedAt: TIMESTAMP,
    });
    createRoutineTriggerMock.mockRejectedValue(new Error("invalid cron expression"));

    renderContent();
    await openCreateDialogToScheduleStep();
    fireEvent.change(screen.getByLabelText(CRON_EXPRESSION_LABEL), {
      target: { value: "bad cron" },
    });
    armFailure();
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "The routine was created without a schedule: invalid cron expression",
      ),
    );
    expect(toast.error).toHaveBeenCalledWith("Failed to load");
  });
});
