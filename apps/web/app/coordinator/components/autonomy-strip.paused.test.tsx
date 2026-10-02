import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AutonomyRead } from "@/lib/api/domains/coordinator-autonomy-api";
import type { AutonomyInput } from "@/hooks/domains/coordinator/use-autonomy";

const flag = vi.hoisted(() => ({ on: true, mobile: false }));
vi.mock("@/hooks/domains/settings/use-coordinator-phase31-effective", () => ({
  useCoordinatorPhase31Effective: () => flag.on,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: flag.mobile, isFinePointer: !flag.mobile }),
}));
vi.mock("@/hooks/domains/coordinator/use-pause-control", () => ({
  usePauseControl: () => ({ pending: false, failed: false, setPaused: vi.fn() }),
}));

import { AutonomyStrip } from "./autonomy-strip";

const STATE = "autonomy-strip-state";
const NOW = Date.parse("2026-09-30T10:00:00Z");

function read(over: Partial<AutonomyRead> = {}): AutonomyRead {
  return {
    server_time: new Date(NOW).toISOString(),
    autonomy_enabled: true,
    admission: { ok: true, detail: "" },
    pending_wakes: 3,
    oldest_pending_at: null,
    last_woke_at: null,
    last_turn: null,
    containment: { conditions: [] },
    spend: {
      measurable: true,
      degraded: false,
      window_subcents: 0,
      mean_daily_subcents_7d: 0,
      mean_known: true,
      ceiling_subcents: 1000,
    },
    paused: true,
    paused_at: "2026-09-30T09:12:00Z",
    paused_by: { id: "u1", name: "Ada" },
    ...over,
  };
}

function input(value: AutonomyRead): AutonomyInput {
  return { value, loadedAt: NOW, error: false, loading: false, retry: vi.fn() };
}

function renderStrip(value: AutonomyRead, canManage = true) {
  return render(
    <AutonomyStrip
      autonomy={input(value)}
      canManage={canManage}
      workspaceId="w1"
      coordinatorId="c1"
    />,
  );
}

afterEach(() => {
  cleanup();
  flag.on = true;
  flag.mobile = false;
});

describe("AutonomyStrip paused", () => {
  it("shows Paused with the manager, keeps the pending count and offers Resume", () => {
    renderStrip(read());
    const state = screen.getByTestId(STATE);
    expect(state.textContent).toContain("Autonomy: Paused by Ada at");
    expect(state.getAttribute("data-state")).toBe("paused");
    expect(screen.getByTestId("autonomy-strip-pending").textContent).toBe("3 pending");
    expect(screen.getByTestId("autonomy-resume")).toBeTruthy();
  });

  it("offers a manager Pause while autonomy is on and the coordinator is not paused", () => {
    renderStrip(read({ paused: false, paused_at: null, paused_by: null }));
    expect(screen.getByTestId("autonomy-pause")).toBeTruthy();
    expect(screen.getByTestId(STATE).textContent).toBe("Autonomy: Active");
  });

  it("shows a reader the state and no control", () => {
    renderStrip(read(), false);
    expect(screen.getByTestId(STATE).getAttribute("data-state")).toBe("paused");
    expect(screen.queryByTestId("autonomy-resume")).toBeNull();
  });

  it("shows a read-only badge with the note and no control while the flag is off", () => {
    flag.on = false;
    renderStrip(read());
    expect(screen.getByTestId(STATE).getAttribute("data-state")).toBe("paused");
    expect(screen.getByTestId("autonomy-paused-flag-off-note").textContent).toBe(
      "Resume needs the phase 3.1 features to be on",
    );
    expect(screen.queryByTestId("autonomy-resume")).toBeNull();
  });

  it("names no manager when the user is unreadable", () => {
    renderStrip(read({ paused_by: null }));
    expect(screen.getByTestId(STATE).textContent).toContain("Paused at");
    expect(screen.getByTestId(STATE).textContent).not.toContain("by");
  });

  it("puts the control on its own row on a phone", () => {
    flag.mobile = true;
    renderStrip(read());
    expect(screen.getByTestId("autonomy-strip-control-row").querySelector("button")).toBeTruthy();
  });

  it("is hidden with autonomy off", () => {
    const { container } = renderStrip(read({ autonomy_enabled: false }));
    expect(container.firstChild).toBeNull();
  });
});
