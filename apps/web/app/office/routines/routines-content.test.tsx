import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { RoutinesContent } from "./routines-content";
import {
  createRoutine,
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
  };
});

import { toast } from "sonner";

const listRoutinesMock = vi.mocked(listRoutines);
const listAllRoutineRunsMock = vi.mocked(listAllRoutineRuns);
const listRoutineTriggersMock = vi.mocked(listRoutineTriggers);
const createRoutineMock = vi.mocked(createRoutine);

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

const CREATED_ROUTINE = {
  id: "routine-1",
  workspaceId: WORKSPACE_ID,
  name: ROUTINE_NAME,
  taskTemplate: {},
  status: "active",
  concurrencyPolicy: "coalesce_if_active",
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
};

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

// AC-002.2 / AC-002.10: the routine and its cron trigger are created in one
// request, so a rejected trigger cannot leave a routine behind.
describe("RoutinesContent create-routine cron arm (AC-002.2, AC-002.10)", () => {
  it("creates the routine and its cron trigger in one request and shows one success toast", async () => {
    listRoutinesMock.mockResolvedValue({ routines: [] });
    listAllRoutineRunsMock.mockResolvedValue({ runs: [] });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
    createRoutineMock.mockResolvedValue(CREATED_ROUTINE);

    renderContent();
    await openCreateDialogToScheduleStep();
    fireEvent.change(screen.getByLabelText(CRON_EXPRESSION_LABEL), {
      target: { value: "0 9 * * *" },
    });
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => {
      expect(createRoutineMock).toHaveBeenCalledWith(
        WORKSPACE_ID,
        expect.objectContaining({
          name: ROUTINE_NAME,
          trigger: { kind: "cron", cronExpression: "0 9 * * *", timezone: "UTC" },
        }),
      );
    });
    expect(createRoutineMock).toHaveBeenCalledTimes(1);
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

  it("keeps the dialog open with an error toast and no success toast when the atomic create fails", async () => {
    listRoutinesMock.mockResolvedValue({ routines: [] });
    listAllRoutineRunsMock.mockResolvedValue({ runs: [] });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
    createRoutineMock.mockRejectedValue(new Error("invalid cron expression"));

    renderContent();
    await openCreateDialogToScheduleStep();
    fireEvent.change(screen.getByLabelText(CRON_EXPRESSION_LABEL), {
      target: { value: "0 9 * * *" },
    });
    const callsBeforeCreate = listRoutinesMock.mock.calls.length;
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("invalid cron expression"));
    expect(toast.success).not.toHaveBeenCalled();
    // Nothing was created, so the list must not be refetched and the dialog
    // must stay open for a retry.
    expect(listRoutinesMock.mock.calls.length).toBe(callsBeforeCreate);
    expect(screen.getByRole("button", { name: /^create$/i })).toBeTruthy();
  });
});

// TS-006: a rejected createRoutine call must leave the dialog open for the
// user to correct and retry, and must not refetch the routine list for a
// routine that was never created.
describe("RoutinesContent create failure (TS-006)", () => {
  it("keeps the dialog open and shows the create error without refetching when createRoutine rejects", async () => {
    listRoutinesMock.mockResolvedValue({ routines: [] });
    listAllRoutineRunsMock.mockResolvedValue({ runs: [] });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
    createRoutineMock.mockRejectedValue(new Error("duplicate name"));

    renderContent();
    await openCreateDialogToScheduleStep();
    fireEvent.change(screen.getByLabelText(CRON_EXPRESSION_LABEL), {
      target: { value: "0 9 * * *" },
    });
    const callsBeforeCreate = listRoutinesMock.mock.calls.length;
    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("duplicate name"));
    expect(toast.success).not.toHaveBeenCalled();
    expect(listRoutinesMock.mock.calls.length).toBe(callsBeforeCreate);
    expect(screen.getByRole("button", { name: /^create$/i })).toBeTruthy();
  });
});

// Regression coverage: the post-create `fetchRoutines()` is guarded, so a
// refetch failure must not swallow the toast reporting the create outcome,
// and must not become an unhandled promise rejection.
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
    createRoutineMock.mockResolvedValue(CREATED_ROUTINE);

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
    expect(createRoutineMock).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.not.objectContaining({ trigger: expect.anything() }),
    );
  });
});
