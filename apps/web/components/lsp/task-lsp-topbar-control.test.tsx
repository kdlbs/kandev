import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";
import { TaskLspTopbarControl } from "./task-lsp-topbar-control";

const TEST_TIME = "2026-08-05T12:00:00Z";
const CONTROL_TEST_ID = "mock-task-lsp-control";

const state = vi.hoisted(() => ({
  appStatusBar: true,
  finePointer: true,
  drawerEnabled: false,
  drawerOpen: false,
  languages: [] as TaskLspLanguageSnapshot[],
  hiddenLanguages: [] as string[],
  openLspStatusDrawer: vi.fn(),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => state.appStatusBar,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: state.finePointer }),
}));

vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  useAppStatusDrawer: () => ({
    enabled: state.drawerEnabled,
    drawerOpen: state.drawerOpen,
    openLspStatusDrawer: state.openLspStatusDrawer,
  }),
}));

vi.mock("@/hooks/domains/lsp/use-task-lsp", () => ({
  useTaskLsp: () => ({ languages: state.languages }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (value: { userSettings: { lspStatusHiddenLanguages: string[] } }) => unknown,
  ) => selector({ userSettings: { lspStatusHiddenLanguages: state.hiddenLanguages } }),
}));

vi.mock("./task-lsp-control", () => ({
  TaskLspControl: (props: Record<string, unknown>) => (
    <button
      data-testid={CONTROL_TEST_ID}
      data-task-id={String(props.taskId)}
      data-touch={String(props.touch)}
      data-external={String(Boolean(props.onOpenExternal))}
    />
  ),
}));

const kotlin: TaskLspLanguageSnapshot = {
  task_id: "task-1",
  language: "kotlin",
  policy: "inherit",
  detected: true,
  detection_state: "complete",
  detection_truncated: false,
  phase: "off",
  generation: 0,
  revision: 1,
  last_transition_at: TEST_TIME,
  last_action: "",
  last_initiator: "automatic",
  restart_required: false,
  created_at: TEST_TIME,
  updated_at: TEST_TIME,
  effective_policy: "inherit",
  activity: "idle",
  progress: [],
};

describe("TaskLspTopbarControl", () => {
  beforeEach(() => {
    state.appStatusBar = true;
    state.finePointer = true;
    state.drawerEnabled = false;
    state.drawerOpen = false;
    state.languages = [kotlin];
    state.hiddenLanguages = [];
    state.openLspStatusDrawer.mockReset();
  });

  afterEach(cleanup);

  it("avoids duplicating a relevant language already shown in the desktop status bar", () => {
    render(<TaskLspTopbarControl taskId="task-1" />);
    expect(screen.queryByTestId(CONTROL_TEST_ID)).toBeNull();
  });

  it("is the always-discoverable desktop fallback when the status bar is disabled", () => {
    state.appStatusBar = false;
    render(<TaskLspTopbarControl taskId="task-1" />);

    expect(screen.getByTestId(CONTROL_TEST_ID).getAttribute("data-task-id")).toBe("task-1");
  });

  it("remains discoverable when no supported project language has been detected", () => {
    state.languages = [];
    render(<TaskLspTopbarControl taskId="task-1" />);
    expect(screen.getByTestId(CONTROL_TEST_ID)).toBeTruthy();
  });

  it("remains discoverable when every relevant language is hidden from task status", () => {
    state.hiddenLanguages = ["kotlin"];
    render(<TaskLspTopbarControl taskId="task-1" />);

    expect(screen.getByTestId(CONTROL_TEST_ID)).toBeTruthy();
  });

  it("delegates coarse tablet presentation into the existing Status drawer", () => {
    state.finePointer = false;
    state.drawerEnabled = true;
    render(<TaskLspTopbarControl taskId="task-1" />);

    const control = screen.getByTestId(CONTROL_TEST_ID);
    expect(control.getAttribute("data-touch")).toBe("true");
    expect(control.getAttribute("data-external")).toBe("true");
  });
});
