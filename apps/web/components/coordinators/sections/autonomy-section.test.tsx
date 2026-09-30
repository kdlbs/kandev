import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { AutonomyRead } from "@/lib/api/domains/coordinator-autonomy-api";

const getCoordinator = vi.fn();
const patchCoordinator = vi.fn();
const getAutonomy = vi.fn();
const toastError = vi.fn();
let contributor: {
  isDirty: boolean;
  canSave: boolean;
  invalidReason?: string;
  save: () => Promise<void>;
  discard: () => void;
} | null = null;

vi.mock("@/lib/toast/sonner", () => ({ toast: { error: (...a: unknown[]) => toastError(...a) } }));
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (c: typeof contributor) => {
    contributor = c;
  },
}));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));
vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return {
    ...actual,
    getCoordinator: (...a: unknown[]) => getCoordinator(...a),
    patchCoordinator: (...a: unknown[]) => patchCoordinator(...a),
  };
});
vi.mock("@/lib/api/domains/coordinator-autonomy-api", () => ({
  getAutonomy: (...a: unknown[]) => getAutonomy(...a),
}));

import { AutonomySection } from "./autonomy-section";

const NOW = "2026-09-30T10:00:00Z";
const CEILING = "autonomy-ceiling";
const TOGGLE = "autonomy-toggle";

function coordinator(over: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "c1",
    workspace_id: "w1",
    name: "P",
    autonomy_enabled: false,
    cost_ceiling_usd: null,
    ...over,
  } as Coordinator;
}

function read(over: Partial<AutonomyRead> = {}): AutonomyRead {
  return {
    server_time: NOW,
    autonomy_enabled: true,
    admission: { ok: true, detail: "" },
    pending_wakes: 0,
    oldest_pending_at: null,
    last_woke_at: null,
    last_turn: null,
    containment: {
      conditions: [
        { name: "executor_isolated", met: true, detail: "docker" },
        { name: "auth_enabled", met: false, detail: "disabled" },
        { name: "no_kandev_credential", met: true, detail: "" },
        { name: "no_extra_tools", met: false, detail: "unreadable" },
      ],
    },
    spend: {
      measurable: true,
      degraded: false,
      window_subcents: 25_000,
      mean_daily_subcents_7d: 12_345,
      mean_known: true,
      ceiling_subcents: 100_000,
    },
    ...over,
  };
}

function renderSection(canManage = true) {
  return render(<AutonomySection workspaceId="w1" coordinatorId="c1" canManage={canManage} />);
}

async function ready() {
  await waitFor(() => expect(screen.getByTestId("autonomy-section")).toBeTruthy());
  await waitFor(() => expect(screen.getByTestId(CONTAINMENT_LIST)).toBeTruthy());
}

function ceilingInput() {
  return screen.getByTestId(CEILING) as HTMLInputElement;
}

function type(value: string) {
  fireEvent.change(ceilingInput(), { target: { value } });
}

beforeEach(() => {
  contributor = null;
  getCoordinator.mockReset().mockResolvedValue(coordinator());
  patchCoordinator.mockReset();
  getAutonomy.mockReset().mockResolvedValue(read());
  toastError.mockReset();
});
afterEach(cleanup);

const CONTAINMENT_LIST = "containment-list";
const CEILING_HINT = "autonomy-ceiling-hint";

describe("AutonomySection - draft and save", () => {
  it("is clean until edited, then saves one PATCH with only the changed keys", async () => {
    patchCoordinator.mockResolvedValue(
      coordinator({ autonomy_enabled: true, cost_ceiling_usd: "10.50" }),
    );
    renderSection();
    await ready();
    expect(contributor?.isDirty).toBe(false);
    fireEvent.click(screen.getByTestId(TOGGLE));
    type("10.5");
    expect(contributor?.isDirty).toBe(true);
    await act(async () => {
      await contributor?.save();
    });
    expect(patchCoordinator).toHaveBeenCalledTimes(1);
    expect(patchCoordinator).toHaveBeenCalledWith("w1", "c1", {
      autonomy_enabled: true,
      cost_ceiling_usd: "10.50",
    });
  });

  it("sends only the toggle when the ceiling is unchanged, comparing as cents", async () => {
    getCoordinator.mockResolvedValue(coordinator({ cost_ceiling_usd: "10.50" }));
    patchCoordinator.mockResolvedValue(
      coordinator({ autonomy_enabled: true, cost_ceiling_usd: "10.50" }),
    );
    renderSection();
    await ready();
    await waitFor(() => expect(ceilingInput().value).toBe("10.50"));
    type("10.5");
    expect(contributor?.isDirty).toBe(false);
    fireEvent.click(screen.getByTestId(TOGGLE));
    await act(async () => {
      await contributor?.save();
    });
    expect(patchCoordinator).toHaveBeenCalledWith("w1", "c1", { autonomy_enabled: true });
  });

  it("clears the ceiling with null", async () => {
    getCoordinator.mockResolvedValue(coordinator({ cost_ceiling_usd: "5.00" }));
    patchCoordinator.mockResolvedValue(coordinator());
    renderSection();
    await ready();
    await waitFor(() => expect(ceilingInput().value).toBe("5.00"));
    type("");
    await act(async () => {
      await contributor?.save();
    });
    expect(patchCoordinator).toHaveBeenCalledWith("w1", "c1", { cost_ceiling_usd: null });
  });
});

describe("AutonomySection - draft validation and failures", () => {
  it("blocks Save with a required hint when the toggle is on and the ceiling is empty", async () => {
    renderSection();
    await ready();
    fireEvent.click(screen.getByTestId(TOGGLE));
    expect(screen.getByTestId(CEILING_HINT).getAttribute("data-hint")).toBe("required");
    expect(contributor?.canSave).toBe(false);
  });

  it.each(["0", "0.00", "10000.01", "abc", "1.234", "-1"])(
    "blocks Save with an invalid hint for %s",
    async (value) => {
      renderSection();
      await ready();
      type(value);
      expect(screen.getByTestId(CEILING_HINT).getAttribute("data-hint")).toBe("invalid");
      expect(contributor?.canSave).toBe(false);
    },
  );

  it("accepts the range bounds", async () => {
    renderSection();
    await ready();
    for (const value of ["0.01", "10000.00"]) {
      type(value);
      expect(screen.queryByTestId(CEILING_HINT)).toBeNull();
      expect(contributor?.canSave).toBe(true);
    }
  });

  it("maps a 400 naming cost_ceiling to the keyed string, never the server text", async () => {
    patchCoordinator.mockRejectedValue(
      new ApiError("server words", 400, { error: "server words", field: "cost_ceiling_usd" }),
    );
    renderSection();
    await ready();
    type("5");
    await act(async () => {
      await contributor?.save().catch(() => undefined);
    });
    const hint = screen.getByTestId(CEILING_HINT);
    expect(hint.getAttribute("data-hint")).toBe("rejected");
    expect(hint.textContent).toBe(
      "That cost ceiling was not accepted. Enter an amount from 0.01 to 10000.00",
    );
    expect(screen.queryByText("server words")).toBeNull();
    expect(toastError).not.toHaveBeenCalled();
  });

  it("reports a non-400 failure through a toast, rejects the save and keeps the draft dirty", async () => {
    patchCoordinator.mockRejectedValue(new ApiError("boom", 500, { error: "boom" }));
    renderSection();
    await ready();
    type("5");
    let failed = false;
    await act(async () => {
      await contributor?.save().catch(() => {
        failed = true;
      });
    });
    expect(failed).toBe(true);
    expect(toastError).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId(CEILING_HINT)).toBeNull();
    expect(ceilingInput().value).toBe("5");
    expect(contributor?.isDirty).toBe(true);
  });

  it("refreshes the coordinator then the autonomy read after a save, and a refresh failure leaves the save successful", async () => {
    patchCoordinator.mockResolvedValue(coordinator({ cost_ceiling_usd: "5.00" }));
    renderSection();
    await ready();
    type("5");
    getCoordinator.mockClear();
    getAutonomy.mockClear();
    getCoordinator.mockRejectedValue(new Error("down"));
    getAutonomy.mockRejectedValue(new Error("down"));
    let failed = false;
    await act(async () => {
      await contributor?.save().catch(() => {
        failed = true;
      });
    });
    expect(failed).toBe(false);
    expect(getCoordinator).toHaveBeenCalledTimes(1);
    expect(getAutonomy).toHaveBeenCalledTimes(1);
    expect(contributor?.isDirty).toBe(false);
    expect(ceilingInput().value).toBe("5.00");
  });
});

describe("AutonomySection - reader, spend and containment", () => {
  it("disables the controls for a reader and marks the section clean", async () => {
    renderSection(false);
    await ready();
    expect((screen.getByTestId(TOGGLE) as HTMLButtonElement).disabled).toBe(true);
    expect(ceilingInput().disabled).toBe(true);
    expect(contributor?.isDirty).toBe(false);
  });
});

describe("AutonomySection - spend and containment", () => {
  it("shows spend, the 7-day mean and the last turn cost", async () => {
    getAutonomy.mockResolvedValue(
      read({
        last_turn: {
          id: "t1",
          coordinator_id: "c1",
          conversation_task_id: "x",
          session_id: "s",
          started_at: NOW,
          finished_at: NOW,
          outcome: "completed",
          wake_count: 1,
          denied_permissions: 0,
          cost_subcents: 4_200,
          stop_requested_at: null,
          stop_state: null,
        },
      }),
    );
    renderSection();
    await ready();
    expect(screen.getByTestId("autonomy-spend").textContent).toContain("2.50 of 10.00 USD");
    expect(screen.getByTestId("autonomy-spend-mean").textContent).toBe("7-day daily mean 1.23 USD");
    expect(screen.getByTestId("autonomy-spend-last").textContent).toBe(
      "Last unattended turn 0.42 USD",
    );
  });

  it("names unknown mean, unknown last-turn cost and no turns", async () => {
    getAutonomy.mockResolvedValue(
      read({
        spend: {
          measurable: true,
          degraded: false,
          window_subcents: 0,
          mean_daily_subcents_7d: null,
          mean_known: false,
          ceiling_subcents: null,
        },
      }),
    );
    renderSection();
    await ready();
    expect(screen.getByTestId("autonomy-spend-mean").textContent).toBe(
      "7-day daily mean unavailable",
    );
    expect(screen.getByTestId("autonomy-spend-last").textContent).toBe("No unattended turns yet");
    expect(screen.getByTestId("autonomy-spend-pill").getAttribute("data-pill")).toBe("no-ceiling");
  });

  it("lists the four conditions in received order with fix text and machine tokens only under Not met", async () => {
    renderSection();
    await ready();
    const rows = Array.from(screen.getByTestId(CONTAINMENT_LIST).children).map((el) =>
      el.getAttribute("data-testid"),
    );
    expect(rows).toEqual([
      "containment-executor_isolated",
      "containment-auth_enabled",
      "containment-no_kandev_credential",
      "containment-no_extra_tools",
    ]);
    expect(screen.getByTestId("containment-status-executor_isolated").textContent).toBe("Met");
    const met = screen.getByTestId("containment-executor_isolated");
    expect(met.getAttribute("data-met")).toBe("true");
    expect(met.querySelector("code")).toBeNull();
    const auth = screen.getByTestId("containment-auth_enabled");
    expect(auth.getAttribute("data-met")).toBe("false");
    expect(auth.querySelector("code")?.textContent).toBe("disabled");
    expect(auth.textContent).toContain("Turn on Kandev authentication");
    const tools = screen.getByTestId("containment-no_extra_tools");
    expect(tools.textContent).toContain("Kandev could not read this setting");
    expect(tools.querySelector("code")).toBeNull();
  });
});

describe("AutonomySection - loading and failures", () => {
  it("shows a skeleton before the first read", async () => {
    getAutonomy.mockReturnValue(new Promise(() => undefined));
    renderSection();
    await waitFor(() => expect(screen.getByTestId("autonomy-section")).toBeTruthy());
    expect(screen.getByTestId("containment-loading")).toBeTruthy();
    expect(screen.queryByTestId(CONTAINMENT_LIST)).toBeNull();
  });

  it("disables Check again while the read is in flight", async () => {
    renderSection();
    await ready();
    let resolve!: (v: AutonomyRead) => void;
    getAutonomy.mockReturnValue(new Promise<AutonomyRead>((r) => (resolve = r)));
    const button = screen.getByTestId("containment-check-again") as HTMLButtonElement;
    fireEvent.click(button);
    await waitFor(() => expect(button.disabled).toBe(true));
    await act(async () => resolve(read()));
    await waitFor(() => expect(button.disabled).toBe(false));
  });

  it("keeps the last list beside one failure surface with Try again", async () => {
    renderSection();
    await ready();
    getAutonomy.mockRejectedValue(new Error("down"));
    fireEvent.click(screen.getByTestId("containment-check-again"));
    await waitFor(() => expect(screen.getByTestId("autonomy-read-error")).toBeTruthy());
    expect(screen.getAllByTestId("autonomy-read-error")).toHaveLength(1);
    expect(screen.getByTestId(CONTAINMENT_LIST)).toBeTruthy();
    getAutonomy.mockResolvedValue(read());
    fireEvent.click(screen.getByTestId("autonomy-read-retry"));
    await waitFor(() => expect(screen.queryByTestId("autonomy-read-error")).toBeNull());
  });

  it("shows a retry when the coordinator baseline cannot be read", async () => {
    getCoordinator.mockRejectedValueOnce(new Error("down"));
    renderSection();
    await waitFor(() => expect(screen.getByTestId("autonomy-settings-error")).toBeTruthy());
    expect(screen.queryByTestId(TOGGLE)).toBeNull();
  });
});
