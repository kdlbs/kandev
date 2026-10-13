import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AutonomyRead, TurnRead } from "@/lib/api/domains/coordinator-autonomy-api";
import type { AutonomyInput } from "@/hooks/domains/coordinator/use-autonomy";

const stopSessionTurn = vi.fn();
vi.mock("@/lib/coordinator/stop-turn", () => ({
  stopSessionTurn: (...args: unknown[]) => stopSessionTurn(...args),
}));

vi.mock("@/hooks/domains/settings/use-coordinator-phase31-effective", () => ({
  useCoordinatorPhase31Effective: () => false,
}));

import { AutonomyStrip } from "./autonomy-strip";

const NOW = Date.parse("2026-09-30T10:00:00Z");
const STATE = "autonomy-strip-state";
const DATA_STATE = "data-state";
const LAST_WOKE = "autonomy-strip-last-woke";
const SPEND_PILL = "autonomy-spend-pill";
const STOP = "autonomy-stop";
const STOP_WARNING = "autonomy-stop-warning";

beforeEach(() => {
  stopSessionTurn.mockReset();
  vi.useFakeTimers({ toFake: ["Date", "setInterval", "clearInterval"] });
  vi.setSystemTime(NOW);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function turn(over: Partial<TurnRead> = {}): TurnRead {
  return {
    id: "t1",
    coordinator_id: "c1",
    conversation_task_id: "k1",
    session_id: "sess-1",
    started_at: "2026-09-30T09:00:00Z",
    finished_at: null,
    outcome: null,
    wake_count: 1,
    denied_permissions: 0,
    cost_subcents: null,
    stop_requested_at: "2026-09-30T09:50:00Z",
    stop_state: "stop_failing",
    ...over,
  };
}

function read(over: Partial<AutonomyRead> = {}): AutonomyRead {
  return {
    server_time: new Date(NOW).toISOString(),
    autonomy_enabled: true,
    admission: { ok: true, detail: "" },
    pending_wakes: 0,
    oldest_pending_at: null,
    last_woke_at: null,
    last_turn: null,
    containment: { conditions: [] },
    spend: {
      measurable: true,
      degraded: false,
      window_subcents: 10_000,
      mean_daily_subcents_7d: 0,
      mean_known: true,
      ceiling_subcents: 100_000,
    },
    ...over,
  };
}

function input(value: AutonomyRead | null, over: Partial<AutonomyInput> = {}): AutonomyInput {
  return {
    value,
    loadedAt: value ? NOW : null,
    error: false,
    loading: false,
    retry: vi.fn(),
    ...over,
  };
}

function renderStrip(autonomy: AutonomyInput, canManage = true) {
  return render(<AutonomyStrip autonomy={autonomy} canManage={canManage} />);
}

describe("AutonomyStrip visibility", () => {
  it("renders nothing before the first read settles", () => {
    const { container } = renderStrip(input(null));
    expect(container.firstChild).toBeNull();
  });

  it("shows only the retry line on error, whatever stale value is held", () => {
    const retry = vi.fn();
    renderStrip(input(read({ pending_wakes: 5 }), { error: true, retry }));
    expect(screen.getByTestId("autonomy-strip").getAttribute("data-strip")).toBe("unavailable");
    expect(screen.getByText("Autonomy state unavailable")).toBeTruthy();
    expect(screen.queryByTestId(STATE)).toBeNull();
    fireEvent.click(screen.getByTestId("autonomy-strip-retry"));
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it("renders nothing while autonomy is off with no failing stop", () => {
    const { container } = renderStrip(input(read({ autonomy_enabled: false })));
    expect(container.firstChild).toBeNull();
  });
});

describe("AutonomyStrip line 1", () => {
  it("shows Active with last woke age, plural pending and an Inside pill", () => {
    renderStrip(input(read({ pending_wakes: 5, last_woke_at: "2026-09-30T07:55:00Z" })));
    expect(screen.getByTestId(STATE).textContent).toBe("Autonomy: Active");
    expect(screen.getByTestId(STATE).getAttribute(DATA_STATE)).toBe("active");
    expect(screen.getByTestId(LAST_WOKE).textContent).toBe("Last woke 2h 5m ago");
    expect(screen.getByTestId("autonomy-strip-pending").textContent).toBe("5 pending");
    expect(screen.getByTestId(SPEND_PILL).getAttribute("data-pill")).toBe("inside");
    expect(screen.getByTestId("autonomy-spend").textContent).toContain(
      "Spend 1.00 of 10.00 USD in 24 h",
    );
  });

  it("says Not woken yet when last_woke_at is null", () => {
    renderStrip(input(read()));
    expect(screen.getByTestId(LAST_WOKE).textContent).toBe("Not woken yet");
  });

  it("ages last woke with the 30-second tick", () => {
    renderStrip(input(read({ last_woke_at: "2026-09-30T09:59:00Z" })));
    expect(screen.getByTestId(LAST_WOKE).textContent).toBe("Last woke 1m ago");
    act(() => {
      vi.advanceTimersByTime(90_000);
    });
    expect(screen.getByTestId(LAST_WOKE).textContent).toBe("Last woke 2m ago");
  });

  it("shows the transient reasons as Active variants with no settings link", () => {
    renderStrip(input(read({ admission: { ok: false, reason: "conversation_busy", detail: "" } })));
    expect(screen.getByTestId(STATE).textContent).toBe(
      "Autonomy: Active (Waiting for the conversation)",
    );
    expect(screen.getByTestId(STATE).getAttribute(DATA_STATE)).toBe("active");
    cleanup();
    const until = "2026-09-30T10:03:00Z";
    renderStrip(input(read({ admission: { ok: false, reason: "cooldown", detail: "", until } })));
    expect(screen.getByTestId(STATE).textContent).toMatch(
      /^Autonomy: Active \(Between turns until .+\)$/,
    );
    expect(screen.queryByText("Open settings")).toBeNull();
  });

  it("shows each persistent hold with its reason text", () => {
    const cases: [string, string, string][] = [
      ["ceiling_reached", "", "Cost ceiling reached"],
      ["spend_unmeasured", "", "Spend cannot be measured"],
      ["no_conversation", "", "Open the copilot once to give it a conversation"],
      ["conversation_unavailable", "", "The conversation stopped. Open the copilot to restart it"],
      [
        "conversation_unavailable",
        "session_not_started",
        "The conversation has not started. Open the copilot to start it",
      ],
      [
        "containment",
        "no_kandev_credential",
        "Containment not in place: No Kandev credential in the environment",
      ],
    ];
    for (const [reason, detail, text] of cases) {
      renderStrip(input(read({ admission: { ok: false, reason, detail } })));
      expect(screen.getByTestId(STATE).textContent).toBe(`Autonomy: Held (${text})`);
      expect(screen.getByTestId(STATE).getAttribute(DATA_STATE)).toBe("held");
      cleanup();
    }
  });

  it("uses the raw detail for a containment condition the client does not know", () => {
    renderStrip(
      input(read({ admission: { ok: false, reason: "containment", detail: "new_thing" } })),
    );
    expect(screen.getByTestId(STATE).textContent).toBe(
      "Autonomy: Held (Containment not in place: new_thing)",
    );
  });

  it("fails closed on an unknown admission reason: distinct from Active, never healthy", () => {
    renderStrip(input(read({ admission: { ok: false, reason: "brand_new_reason", detail: "" } })));
    const state = screen.getByTestId(STATE);
    expect(state.getAttribute(DATA_STATE)).toBe("unknown");
    expect(state.textContent).toBe("Autonomy: Held (State not recognized)");
    expect(state.textContent).not.toContain("Active");
  });
});

describe("AutonomyStrip spend", () => {
  const base = read().spend;
  it("shows Over at the ceiling", () => {
    renderStrip(input(read({ spend: { ...base, window_subcents: 100_000 } })));
    expect(screen.getByTestId(SPEND_PILL).getAttribute("data-pill")).toBe("over");
  });
  it("shows unknown for degraded and unavailable otherwise, with no pill", () => {
    renderStrip(
      input(read({ spend: { ...base, measurable: false, degraded: true, window_subcents: null } })),
    );
    expect(screen.getByText("Spend unknown: some usage is unpriced")).toBeTruthy();
    expect(screen.queryByTestId(SPEND_PILL)).toBeNull();
    cleanup();
    renderStrip(input(read({ spend: { ...base, measurable: false, window_subcents: null } })));
    expect(screen.getByText("Spend unavailable")).toBeTruthy();
    expect(screen.queryByTestId(SPEND_PILL)).toBeNull();
  });
  it("shows the amount alone, with no pill, when the ceiling is missing", () => {
    renderStrip(input(read({ spend: { ...base, ceiling_subcents: null } })));
    expect(screen.getByTestId("autonomy-spend").textContent).toBe("Spend 1.00 USD in 24 h");
    expect(screen.queryByTestId(SPEND_PILL)).toBeNull();
  });
});

describe("AutonomyStrip stop warning", () => {
  it("shows the warning and Stop for a manager, beside a held line", () => {
    renderStrip(
      input(
        read({
          admission: { ok: false, reason: "ceiling_reached", detail: "" },
          last_turn: turn(),
        }),
      ),
    );
    expect(screen.getByTestId(STATE).getAttribute(DATA_STATE)).toBe("held");
    expect(screen.getByTestId(STOP_WARNING).textContent).toContain(
      "Stop at ceiling not confirmed: the turn is still running",
    );
    expect(screen.getByTestId(STOP)).toBeTruthy();
  });

  it("shows the warning without Stop for a reader", () => {
    renderStrip(input(read({ last_turn: turn() })), false);
    expect(screen.getByTestId(STOP_WARNING)).toBeTruthy();
    expect(screen.queryByTestId(STOP)).toBeNull();
  });

  it("does not show the warning when the stop is not failing", () => {
    renderStrip(input(read({ last_turn: turn({ stop_state: null }) })));
    expect(screen.queryByTestId(STOP_WARNING)).toBeNull();
  });

  it("renders Off plus the warning and Stop only, with no other text, while autonomy is off", () => {
    renderStrip(
      input(
        read({
          autonomy_enabled: false,
          admission: undefined,
          pending_wakes: 4,
          last_turn: turn(),
        }),
      ),
    );
    expect(screen.getByTestId(STATE).textContent).toBe("Autonomy: Off");
    expect(screen.getByTestId(STOP)).toBeTruthy();
    expect(screen.queryByTestId("autonomy-strip-pending")).toBeNull();
    expect(screen.queryByTestId(LAST_WOKE)).toBeNull();
    expect(screen.queryByTestId("autonomy-spend")).toBeNull();
  });

  it("calls the session cancel with last_turn.session_id and is disabled while in flight", async () => {
    let resolve!: () => void;
    stopSessionTurn.mockReturnValue(new Promise<void>((r) => (resolve = r)));
    renderStrip(input(read({ last_turn: turn() })));
    const stop = screen.getByTestId(STOP) as HTMLButtonElement;
    fireEvent.click(stop);
    expect(stopSessionTurn).toHaveBeenCalledWith("sess-1");
    expect((screen.getByTestId(STOP) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId(STOP));
    expect(stopSessionTurn).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolve();
    });
  });

  it("keeps the warning row after a successful cancel while the turn stays open", async () => {
    stopSessionTurn.mockResolvedValue(undefined);
    renderStrip(input(read({ last_turn: turn() })));
    fireEvent.click(screen.getByTestId(STOP));
    await act(async () => {});
    expect(screen.getByTestId(STOP_WARNING)).toBeTruthy();
    expect(screen.queryByTestId("autonomy-stop-failed")).toBeNull();
    expect((screen.getByTestId(STOP) as HTMLButtonElement).disabled).toBe(false);
  });

  it("shows the keyed failure text and re-enables Stop when the cancel fails", async () => {
    stopSessionTurn.mockRejectedValue(new Error("cancel already in flight"));
    renderStrip(input(read({ last_turn: turn() })));
    fireEvent.click(screen.getByTestId(STOP));
    await act(async () => {});
    expect(screen.getByTestId("autonomy-stop-failed").textContent).toBe(
      "Could not stop the turn. Try again.",
    );
    expect(screen.queryByText("cancel already in flight")).toBeNull();
    expect((screen.getByTestId(STOP) as HTMLButtonElement).disabled).toBe(false);
  });
});
