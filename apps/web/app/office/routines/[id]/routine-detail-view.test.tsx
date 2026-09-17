import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import type { Routine, RoutineTrigger } from "@/lib/state/slices/office/types";
import {
  OfficeTopbarChromeProvider,
  useOfficeTopbarChrome,
} from "../../components/office-topbar-context";
import { RoutineDetailView } from "./routine-detail-view";

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn(), replace: vi.fn() }),
}));

const updateRoutineMock = vi.fn().mockResolvedValue({});
const runRoutineMock = vi.fn().mockResolvedValue({});
const createRoutineTriggerMock =
  vi.fn<(routineId: string, data: unknown) => Promise<{ trigger: RoutineTrigger | null }>>();
const deleteRoutineTriggerMock = vi.fn<(triggerId: string) => Promise<void>>();
const listRoutineTriggersMock =
  vi.fn<(routineId: string) => Promise<{ triggers: RoutineTrigger[] }>>();

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    updateRoutine: (...args: [string, unknown]) => updateRoutineMock(...args),
    runRoutine: (...args: [string]) => runRoutineMock(...args),
    createRoutineTrigger: (...args: [string, unknown]) => createRoutineTriggerMock(...args),
    deleteRoutineTrigger: (...args: [string]) => deleteRoutineTriggerMock(...args),
    listRoutineTriggers: (...args: [string]) => listRoutineTriggersMock(...args),
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

const DEFAULT_CRON_EXPRESSION = "*/5 * * * *";
const CHANGED_CRON_EXPRESSION = "0 9 * * *";
const NEXT_FIRE_NONE = "Next fire: -";
const LAST_FIRED_NEVER = "Last fired: never";
const AMERICA_NEW_YORK = "America/New_York";

const cronTrigger: RoutineTrigger = {
  id: "trigger-1",
  routineId: "routine-1",
  kind: "cron",
  cronExpression: DEFAULT_CRON_EXPRESSION,
  timezone: "UTC",
  nextRunAt: "2026-05-05T00:00:00Z",
  enabled: true,
  createdAt: TIMESTAMP,
  updatedAt: TIMESTAMP,
};

const NO_TRIGGERS: RoutineTrigger[] = [];

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

function clickSave() {
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
}

function setCronExpression(value: string) {
  fireEvent.change(screen.getByPlaceholderText(DEFAULT_CRON_EXPRESSION), { target: { value } });
}

function setTimezone(value: string) {
  fireEvent.change(screen.getByPlaceholderText("UTC"), { target: { value } });
}

function setTriggerKindWebhook() {
  const kindField = screen.getByText("Kind").closest("div") as HTMLElement;
  fireEvent.click(within(kindField).getByRole("combobox"));
  const listbox = screen.getByRole("listbox");
  fireEvent.click(within(listbox).getByRole("option", { name: "Webhook" }));
}

describe("RoutineDetailView next-fire display", () => {
  it("hides the next-fire countdown as soon as status is changed to paused, before saving", async () => {
    renderDetailView(baseRoutine, [cronTrigger]);

    expect(screen.getByText(/Next fire: 5\/5\/2026/)).toBeTruthy();

    const statusField = screen.getByText("Status").closest("div") as HTMLElement;
    fireEvent.click(within(statusField).getByRole("combobox"));
    const listbox = await screen.findByRole("listbox");
    fireEvent.click(within(listbox).getByRole("option", { name: "Paused" }));

    expect(screen.getByText(NEXT_FIRE_NONE)).toBeTruthy();
    expect(screen.queryByText(/Next fire: 5\/5\/2026/)).toBeNull();
  });

  it("keeps showing the next-fire countdown for a routine loaded as active", () => {
    renderDetailView(baseRoutine, [cronTrigger]);
    expect(screen.getByText(/Next fire: 5\/5\/2026/)).toBeTruthy();
  });

  it("shows no next-fire countdown for a routine loaded as paused", () => {
    renderDetailView({ ...baseRoutine, status: "paused" }, [cronTrigger]);
    expect(screen.getByText(NEXT_FIRE_NONE)).toBeTruthy();
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

describe("RoutineDetailView editable field seeding (AC-003.5, AC-003.7)", () => {
  it("seeds the editable cron expression and timezone from the primary trigger, not array position", () => {
    const other = { ...cronTrigger, id: "b", nextRunAt: undefined, enabled: false };
    const primary = {
      ...cronTrigger,
      id: "a",
      cronExpression: CHANGED_CRON_EXPRESSION,
      timezone: AMERICA_NEW_YORK,
    };
    // Primary is listed second: seeding must not pick by array position.
    renderDetailView(baseRoutine, [other, primary]);
    expect(screen.getByDisplayValue(CHANGED_CRON_EXPRESSION)).toBeTruthy();
    expect(screen.getByDisplayValue(AMERICA_NEW_YORK)).toBeTruthy();
  });

  it("seeds an empty cron expression and the default timezone when there is no cron trigger", () => {
    renderDetailView(baseRoutine, NO_TRIGGERS);
    expect((screen.getByPlaceholderText(DEFAULT_CRON_EXPRESSION) as HTMLInputElement).value).toBe(
      "",
    );
    expect(screen.getByDisplayValue("UTC")).toBeTruthy();
  });
});

describe("RoutineDetailView last-fired display (AC-003.5, AC-003.6)", () => {
  it("shows the primary cron trigger's lastFiredAt, not the placeholder, when one is set", () => {
    renderDetailView(baseRoutine, [{ ...cronTrigger, lastFiredAt: "2026-05-01T00:00:00Z" }]);
    expect(screen.queryByText(LAST_FIRED_NEVER)).toBeNull();
    expect(screen.getByText(/^Last fired: (?!never)/)).toBeTruthy();
  });

  it("shows the never placeholder when there is no cron trigger to report a last-fired time", () => {
    renderDetailView(baseRoutine, NO_TRIGGERS);
    expect(screen.getByText(LAST_FIRED_NEVER)).toBeTruthy();
  });

  it("shows the not-first-in-array primary trigger's lastFiredAt, not the placeholder array position would surface", () => {
    const other = { ...cronTrigger, id: "b", nextRunAt: undefined, enabled: false };
    const primary = { ...cronTrigger, id: "a", lastFiredAt: "2026-05-01T00:00:00Z" };
    // Primary is listed second: reading by array position would surface
    // `other`'s unset lastFiredAt (the never placeholder) instead.
    renderDetailView(baseRoutine, [other, primary]);
    expect(screen.queryByText(LAST_FIRED_NEVER)).toBeNull();
    expect(screen.getByText(/^Last fired: (?!never)/)).toBeTruthy();
  });

  it("shows last-fired even when the routine is not currently firing", () => {
    renderDetailView({ ...baseRoutine, status: "paused" }, [
      { ...cronTrigger, lastFiredAt: "2026-05-01T00:00:00Z" },
    ]);
    expect(screen.queryByText(LAST_FIRED_NEVER)).toBeNull();
    expect(screen.getByText(/^Last fired: (?!never)/)).toBeTruthy();
  });
});

describe("RoutineDetailView next-fire isPrimary gate (AC-003.5)", () => {
  it("hides a fallback (non-primary) cron trigger's stale nextRunAt, even though the routine is active", () => {
    const fallbackOnly: RoutineTrigger = {
      ...cronTrigger,
      enabled: false,
      nextRunAt: "2020-01-01T00:00:00Z",
    };
    renderDetailView(baseRoutine, [fallbackOnly]);
    expect(screen.getByText(NEXT_FIRE_NONE)).toBeTruthy();
    expect(screen.queryByText(/Next fire: 1\/1\/2020/)).toBeNull();
  });
});

describe("RoutineDetailView trigger sync (REQ-004)", () => {
  it("issues neither delete nor create when the draft matches the sync target (AC-004.4)", async () => {
    renderDetailView(baseRoutine, [cronTrigger]);
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).not.toHaveBeenCalled();
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
  });

  it("deletes then creates when the drafted expression changes, replacing the trigger (AC-004.3)", async () => {
    const NEW_NEXT_RUN_AT = "2026-06-06T00:00:00Z";
    createRoutineTriggerMock.mockResolvedValue({
      trigger: {
        ...cronTrigger,
        id: "trigger-2",
        cronExpression: CHANGED_CRON_EXPRESSION,
        nextRunAt: NEW_NEXT_RUN_AT,
      },
    });
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).toHaveBeenCalledWith("trigger-1");
    expect(createRoutineTriggerMock).toHaveBeenCalledWith(
      "routine-1",
      expect.objectContaining({
        kind: "cron",
        cronExpression: CHANGED_CRON_EXPRESSION,
        timezone: "UTC",
      }),
    );
    // The card renders the newly created trigger's nextRunAt, not the
    // deleted trigger's, proving displayed state was actually replaced.
    expect(screen.getByText(/Next fire: 6\/6\/2026/)).toBeTruthy();
    expect(screen.queryByText(/Next fire: 5\/5\/2026/)).toBeNull();
  });

  it("reports the no-schedule error and drops the deleted trigger when create fails and no cron trigger remains (AC-004.1, AC-004.2)", async () => {
    deleteRoutineTriggerMock.mockResolvedValue(undefined);
    createRoutineTriggerMock.mockRejectedValue(new Error("boom"));
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).toHaveBeenCalledWith("trigger-1");
    expect(toastError).toHaveBeenCalledWith(
      "Schedule not saved. This routine now has no cron schedule",
    );
    // The deleted trigger's next-fire value is actually gone from displayed
    // state, not just the delete call having been issued.
    expect(screen.getByText(NEXT_FIRE_NONE)).toBeTruthy();
    expect(screen.queryByText(/Next fire: 5\/5\/2026/)).toBeNull();
  });

  it("reports the kept-previous error when create fails but another cron trigger remains (AC-004.2, AC-004.7)", async () => {
    const other: RoutineTrigger = { ...cronTrigger, id: "trigger-9" };
    deleteRoutineTriggerMock.mockResolvedValue(undefined);
    createRoutineTriggerMock.mockRejectedValue(new Error("boom"));
    renderDetailView(baseRoutine, [cronTrigger, other]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(toastError).toHaveBeenCalledWith(
      "New schedule not saved. The routine is still on its previous schedule",
    );
    // Sync targets exactly one cron trigger (AC-004.8): the other cron
    // trigger was never touched.
    expect(deleteRoutineTriggerMock).toHaveBeenCalledTimes(1);
    expect(deleteRoutineTriggerMock).toHaveBeenCalledWith("trigger-1");
  });

  it("does not call create and reports the generic save failure when the delete itself fails (AC-004.5)", async () => {
    deleteRoutineTriggerMock.mockRejectedValue(new Error("delete boom"));
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("delete boom");
  });

  it("targets the primary trigger for sync, not the first array element, when the primary is listed second (AC-004.8)", async () => {
    const other: RoutineTrigger = { ...cronTrigger, id: "b", nextRunAt: undefined, enabled: false };
    const primary: RoutineTrigger = { ...cronTrigger, id: "a" };
    deleteRoutineTriggerMock.mockResolvedValue(undefined);
    createRoutineTriggerMock.mockResolvedValue({
      trigger: { ...cronTrigger, id: "trigger-new", cronExpression: CHANGED_CRON_EXPRESSION },
    });
    renderDetailView(baseRoutine, [other, primary]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).toHaveBeenCalledWith("a");
    expect(deleteRoutineTriggerMock).not.toHaveBeenCalledWith("b");
  });
});

describe("RoutineDetailView trigger sync: unusable create and refetch (AC-004.6, AC-004.9)", () => {
  it("refetches and uses the refetched cron trigger when create succeeds but returns no usable trigger (AC-004.6)", async () => {
    const REFETCHED_NEXT_RUN_AT = "2026-07-07T00:00:00Z";
    deleteRoutineTriggerMock.mockResolvedValue(undefined);
    createRoutineTriggerMock.mockResolvedValue({ trigger: null });
    listRoutineTriggersMock.mockResolvedValue({
      triggers: [
        {
          ...cronTrigger,
          id: "trigger-3",
          cronExpression: CHANGED_CRON_EXPRESSION,
          nextRunAt: REFETCHED_NEXT_RUN_AT,
        },
      ],
    });
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(listRoutineTriggersMock).toHaveBeenCalledWith("routine-1");
    expect(toastError).not.toHaveBeenCalled();
    // The refetched trigger is actually used to render, not just fetched.
    expect(screen.getByText(/Next fire: 7\/7\/2026/)).toBeTruthy();
    expect(screen.queryByText(/Next fire: 5\/5\/2026/)).toBeNull();
  });

  it("treats an empty refetch as a failed read-back rather than an authoritative empty result (AC-004.6, AC-004.9)", async () => {
    deleteRoutineTriggerMock.mockResolvedValue(undefined);
    createRoutineTriggerMock.mockResolvedValue({ trigger: null });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [] });
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(toastError).toHaveBeenCalledWith(
      "The routine's schedule could not be read back. Reload the page to see its current schedule",
    );
  });

  it("reports a read-back failure when the AC-004.6 refetch itself fails (AC-004.9)", async () => {
    deleteRoutineTriggerMock.mockResolvedValue(undefined);
    createRoutineTriggerMock.mockResolvedValue({ trigger: null });
    listRoutineTriggersMock.mockRejectedValue(new Error("network down"));
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(toastError).toHaveBeenCalledWith(
      "The routine's schedule could not be read back. Reload the page to see its current schedule",
    );
  });
});

describe("RoutineDetailView trigger sync: no target, trimming, timezone (AC-004.10-004.12)", () => {
  it("creates directly with no delete when the routine has no cron trigger yet (AC-004.10)", async () => {
    createRoutineTriggerMock.mockResolvedValue({ trigger: cronTrigger });
    renderDetailView(baseRoutine, NO_TRIGGERS);
    setCronExpression(DEFAULT_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).not.toHaveBeenCalled();
    expect(createRoutineTriggerMock).toHaveBeenCalledWith(
      "routine-1",
      expect.objectContaining({ kind: "cron", cronExpression: DEFAULT_CRON_EXPRESSION }),
    );
    // The newly created trigger's nextRunAt is actually rendered, not just
    // requested from the API.
    expect(screen.getByText(/Next fire: 5\/5\/2026/)).toBeTruthy();
  });

  it("reports the generic save failure, not an AC-004.2 message, when arming the first schedule fails (AC-004.10)", async () => {
    createRoutineTriggerMock.mockRejectedValue(new Error("create boom"));
    renderDetailView(baseRoutine, NO_TRIGGERS);
    setCronExpression(DEFAULT_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(toastError).toHaveBeenCalledWith("create boom");
  });

  it("does not sync when the drafted expression is only whitespace (AC-004.11)", async () => {
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression("   ");
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).not.toHaveBeenCalled();
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
  });

  it("serializes the drafted expression untrimmed once trimming has allowed sync to run (AC-004.11)", async () => {
    createRoutineTriggerMock.mockResolvedValue({ trigger: cronTrigger });
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression("  0 9 * * *  ");
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(createRoutineTriggerMock).toHaveBeenCalledWith(
      "routine-1",
      expect.objectContaining({ cronExpression: "  0 9 * * *  " }),
    );
  });

  it("resolves an empty drafted timezone to the AC-003.7 default before comparing, avoiding a spurious delete+create (AC-004.12)", async () => {
    renderDetailView(baseRoutine, [{ ...cronTrigger, timezone: "UTC" }]);
    setTimezone("");
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).not.toHaveBeenCalled();
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
  });

  it("does not sync when the drafted kind is webhook, even though the drafted expression differs from the trigger's (AC-004.11)", async () => {
    renderDetailView(baseRoutine, [cronTrigger]);
    setCronExpression(CHANGED_CRON_EXPRESSION);
    setTriggerKindWebhook();
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).not.toHaveBeenCalled();
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
  });

  it("does not default an empty stored timezone before comparing, so a real difference still triggers sync (AC-004.4)", async () => {
    const NEW_NEXT_RUN_AT = "2026-06-06T00:00:00Z";
    createRoutineTriggerMock.mockResolvedValue({
      trigger: {
        ...cronTrigger,
        id: "trigger-2",
        timezone: AMERICA_NEW_YORK,
        nextRunAt: NEW_NEXT_RUN_AT,
      },
    });
    renderDetailView(baseRoutine, [{ ...cronTrigger, timezone: "" }]);
    setTimezone(AMERICA_NEW_YORK);
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).toHaveBeenCalledWith("trigger-1");
    expect(createRoutineTriggerMock).toHaveBeenCalledWith(
      "routine-1",
      expect.objectContaining({ timezone: AMERICA_NEW_YORK }),
    );
  });

  it("does not default an empty stored timezone before comparing, even when the drafted timezone also resolves to the default (AC-004.4, AC-004.12)", async () => {
    createRoutineTriggerMock.mockResolvedValue({
      trigger: { ...cronTrigger, id: "trigger-2", timezone: "UTC" },
    });
    // Stored "" and drafted "" both resolve to the same "UTC" reading once
    // AC-004.12's draft-side default is applied. If the stored side were
    // defaulted too, "UTC" === "UTC" would wrongly skip the sync.
    renderDetailView(baseRoutine, [{ ...cronTrigger, timezone: "" }]);
    setTimezone("");
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(deleteRoutineTriggerMock).toHaveBeenCalledWith("trigger-1");
    expect(createRoutineTriggerMock).toHaveBeenCalledWith(
      "routine-1",
      expect.objectContaining({ timezone: "UTC" }),
    );
  });
});

describe("RoutineDetailView draft seeding stability across saves (AC-003.7)", () => {
  it("does not re-seed other draft fields when a save changes the triggers array", async () => {
    createRoutineTriggerMock.mockResolvedValue({
      trigger: { ...cronTrigger, id: "trigger-2", cronExpression: CHANGED_CRON_EXPRESSION },
    });
    renderDetailView(baseRoutine, [cronTrigger]);
    fireEvent.change(screen.getByDisplayValue(baseRoutine.name), {
      target: { value: "Renamed nightly sync" },
    });
    setCronExpression(CHANGED_CRON_EXPRESSION);
    clickSave();
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(screen.getByDisplayValue("Renamed nightly sync")).toBeTruthy();
  });
});
