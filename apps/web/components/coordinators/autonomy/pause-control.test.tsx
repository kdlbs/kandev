import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AutonomyRead } from "@/lib/api/domains/coordinator-autonomy-api";
import type { AutonomyInput } from "@/hooks/domains/coordinator/use-autonomy";

const flag = vi.hoisted(() => ({ on: true, mobile: false }));
const setPaused = vi.fn();
vi.mock("@/hooks/domains/settings/use-coordinator-phase31-effective", () => ({
  useCoordinatorPhase31Effective: () => flag.on,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: flag.mobile, isFinePointer: !flag.mobile }),
}));
const pauseState = vi.hoisted(() => ({ pending: false, failed: false }));
vi.mock("@/hooks/domains/coordinator/use-pause-control", () => ({
  usePauseControl: () => ({ ...pauseState, setPaused }),
}));

import { PauseControl, PausedFlagOffNote } from "./pause-control";

function autonomy(paused: boolean | undefined): AutonomyInput {
  return {
    value: { paused } as unknown as AutonomyRead,
    loadedAt: 1,
    error: false,
    loading: false,
    retry: vi.fn(),
  };
}

function renderControl(a: AutonomyInput, canManage = true) {
  return render(
    <PauseControl workspaceId="w1" coordinatorId="c1" autonomy={a} canManage={canManage} />,
  );
}

const PAUSE = "autonomy-pause";

afterEach(() => {
  cleanup();
  flag.on = true;
  flag.mobile = false;
  pauseState.pending = false;
  pauseState.failed = false;
  setPaused.mockReset();
});

describe("PauseControl", () => {
  it("offers Pause when not paused and Resume when paused", () => {
    renderControl(autonomy(false));
    fireEvent.click(screen.getByTestId(PAUSE));
    expect(setPaused).toHaveBeenCalledWith(true);
    cleanup();
    renderControl(autonomy(true));
    fireEvent.click(screen.getByTestId("autonomy-resume"));
    expect(setPaused).toHaveBeenLastCalledWith(false);
  });

  it("shows no control to a reader or with the flag off", () => {
    renderControl(autonomy(false), false);
    expect(screen.queryByTestId(PAUSE)).toBeNull();
    cleanup();
    flag.on = false;
    renderControl(autonomy(true));
    expect(screen.queryByTestId("autonomy-resume")).toBeNull();
  });

  it("disables while in flight and shows the failure", () => {
    pauseState.pending = true;
    pauseState.failed = true;
    renderControl(autonomy(false));
    expect((screen.getByTestId(PAUSE) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("autonomy-pause-failed").textContent).toBe(
      "Could not change the pause state. Try again.",
    );
  });

  it("is full width with a 44 px target on a phone", () => {
    flag.mobile = true;
    renderControl(autonomy(false));
    const button = screen.getByTestId(PAUSE);
    expect(button.className).toContain("w-full");
    expect(button.className).toContain("min-h-11");
  });
});

describe("PausedFlagOffNote", () => {
  it("shows the note only while paused with the flag off", () => {
    flag.on = false;
    render(<PausedFlagOffNote paused />);
    expect(screen.getByTestId("autonomy-paused-flag-off-note").textContent).toBe(
      "Resume needs the phase 3.1 features to be on",
    );
    cleanup();
    render(<PausedFlagOffNote paused={false} />);
    expect(screen.queryByTestId("autonomy-paused-flag-off-note")).toBeNull();
    cleanup();
    flag.on = true;
    render(<PausedFlagOffNote paused />);
    expect(screen.queryByTestId("autonomy-paused-flag-off-note")).toBeNull();
  });
});
