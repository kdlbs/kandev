import { createElement } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MRTopbarButton } from "./mr-topbar-button";
import type { TaskMR } from "@/lib/types/gitlab";

const OPEN_DELAY_MS = 150;
const TRIGGER_TESTID = "mr-topbar-button";
const POPOVER_STUB_TESTID = "mr-ci-popover-stub";

const gitlabMocks = vi.hoisted(() => ({ mrs: [] as TaskMR[] }));
const touchMocks = vi.hoisted(() => ({ usesTouchDrawer: false }));
const dockviewMocks = vi.hoisted(() => ({ addMRPanel: vi.fn(), isRestoringLayout: false }));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (
    selector: (state: {
      addMRPanel: typeof dockviewMocks.addMRPanel;
      api: object;
      isRestoringLayout: boolean;
    }) => unknown,
  ) =>
    selector({
      addMRPanel: dockviewMocks.addMRPanel,
      api: {},
      isRestoringLayout: dockviewMocks.isRestoringLayout,
    }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      tasks: { activeTaskId: "task-1" },
      workspaces: { activeId: "workspace-1" },
      repositories: { itemsByWorkspaceId: { "workspace-1": [] } },
      removeTaskMR: vi.fn(),
    }),
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: () => true,
  useTaskMRs: () => gitlabMocks.mrs,
  useWorkspaceMRs: vi.fn(),
}));

vi.mock("@/hooks/domains/kanban/use-task-by-id", () => ({
  useTaskById: () => ({ id: "task-1", repositories: [] }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("./task-mr-link-dialog", () => ({ TaskMRLinkDialog: () => null }));
vi.mock("./mr-automation-controls", () => ({ MRAutomationControls: () => null }));
vi.mock("./mr-ci-popover", () => ({
  MRCIPopover: ({ mr }: { mr: TaskMR }) => (
    <div data-testid="mr-ci-popover-stub">popover for !{mr.mr_iid}</div>
  ),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchMocks.usesTouchDrawer,
}));

function singleMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "association-1",
    task_id: "task-1",
    host: "https://gitlab.example",
    project_path: "group/project",
    mr_iid: 81,
    mr_url: "https://gitlab.example/group/project/-/merge_requests/81",
    mr_title: "Test MR",
    head_branch: "feature",
    base_branch: "main",
    author_username: "alice",
    state: "opened",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    reviewer_count: 0,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  } as TaskMR;
}

describe("MRTopbarButton hover preview", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    touchMocks.usesTouchDrawer = false;
    dockviewMocks.addMRPanel.mockClear();
    dockviewMocks.isRestoringLayout = false;
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("AC23: opens the popover after hovering the trigger past the open delay", () => {
    gitlabMocks.mrs = [singleMR()];
    render(createElement(MRTopbarButton));
    const trigger = screen.getByTestId(TRIGGER_TESTID);

    expect(screen.queryByTestId(POPOVER_STUB_TESTID)).toBeNull();
    act(() => {
      fireEvent.mouseEnter(trigger);
    });
    expect(screen.queryByTestId("mr-topbar-popover")).toBeNull();
    act(() => {
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });
    expect(screen.getByTestId(POPOVER_STUB_TESTID).textContent).toContain("!81");

    act(() => {
      fireEvent.mouseLeave(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });
    expect(screen.queryByTestId(POPOVER_STUB_TESTID)).toBeNull();
  });

  it("AC25: clicking the trigger opens the MR detail panel directly, closing any open hover popover", () => {
    gitlabMocks.mrs = [singleMR()];
    render(createElement(MRTopbarButton));
    const trigger = screen.getByTestId(TRIGGER_TESTID);

    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });
    expect(screen.getByTestId(POPOVER_STUB_TESTID)).toBeTruthy();

    fireEvent.pointerDown(trigger, { button: 0, pointerId: 1, pointerType: "mouse" });
    fireEvent.pointerUp(trigger, { button: 0, pointerId: 1, pointerType: "mouse" });
    fireEvent.click(trigger);
    // Single-MR desktop click has no dropdown to open — it goes straight to
    // the MR detail panel (mirrors GitHub's PRSingleButton), so there is no
    // aria-expanded dropdown state to assert here, only the panel call and
    // the hover popover closing.
    expect(dockviewMocks.addMRPanel).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId(POPOVER_STUB_TESTID)).toBeNull();
  });

  it("keeps a direct click pending until the Dockview layout has settled", () => {
    dockviewMocks.isRestoringLayout = true;
    gitlabMocks.mrs = [singleMR()];
    const view = render(createElement(MRTopbarButton));
    const trigger = screen.getByTestId(TRIGGER_TESTID);

    act(() => {
      fireEvent.click(trigger);
    });
    expect(dockviewMocks.addMRPanel).not.toHaveBeenCalled();

    dockviewMocks.isRestoringLayout = false;
    act(() => {
      view.rerender(createElement(MRTopbarButton, { compact: true }));
    });
    expect(dockviewMocks.addMRPanel).toHaveBeenCalledTimes(1);
  });

  it("AC26: on touch, no hover popover is ever rendered and the dropdown still opens on click", () => {
    touchMocks.usesTouchDrawer = true;
    gitlabMocks.mrs = [singleMR()];
    render(createElement(MRTopbarButton));
    const trigger = screen.getByTestId(TRIGGER_TESTID);

    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS * 2);
    });
    expect(screen.queryByTestId(POPOVER_STUB_TESTID)).toBeNull();
    expect(screen.queryByTestId("mr-topbar-popover")).toBeNull();

    fireEvent.pointerDown(trigger, { button: 0, pointerId: 1, pointerType: "touch" });
    fireEvent.pointerUp(trigger, { button: 0, pointerId: 1, pointerType: "touch" });
    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
  });

  it("does not render a hover popover for the multi-MR case", () => {
    touchMocks.usesTouchDrawer = false;
    gitlabMocks.mrs = [singleMR({ id: "a", mr_iid: 1 }), singleMR({ id: "b", mr_iid: 2 })];
    render(createElement(MRTopbarButton));
    const trigger = screen.getByTestId(TRIGGER_TESTID);

    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS * 2);
    });
    expect(screen.queryByTestId(POPOVER_STUB_TESTID)).toBeNull();
  });
});
