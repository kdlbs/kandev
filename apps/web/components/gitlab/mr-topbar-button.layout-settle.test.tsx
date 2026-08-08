/**
 * Regression cover for the MR detail panel being silently discarded when the
 * user clicks the topbar button while a task's session is still loading.
 *
 * Navigating straight to /t/:id runs dockview's `onReady` before the
 * session→env mapping hydrates, so it builds a throwaway DEFAULT layout with a
 * null env; `switchEnvLayout` then replaces that layout wholesale via
 * `api.fromJSON` once the env arrives. A panel added in that gap is thrown
 * away and the click appears to do nothing. `useMRDesktopReviewOpener` must
 * therefore re-apply the open once the env's layout has settled.
 */
import { createElement, useSyncExternalStore } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MRTopbarButton } from "./mr-topbar-button";
import type { TaskMR } from "@/lib/types/gitlab";

const TRIGGER_TESTID = "mr-topbar-button";

const gitlabMocks = vi.hoisted(() => ({ mrs: [] as TaskMR[] }));
// A genuinely reactive stand-in for the dockview store: MRTopbarButton is
// memo()-wrapped, so a plain rerender with identical props would never re-read
// module-level mock values. Subscribers make store changes drive the re-render
// the same way zustand does in the app.
const dockviewMocks = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  const state = {
    addMRPanel: vi.fn(),
    api: {} as object | null,
    currentLayoutEnvId: null as string | null,
    isRestoringLayout: false,
  };
  return {
    state,
    listeners,
    get addMRPanel() {
      return state.addMRPanel;
    },
    set(patch: Partial<typeof state>) {
      Object.assign(state, patch);
      listeners.forEach((l) => l());
    },
  };
});

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: unknown) => unknown) =>
    useSyncExternalStore(
      (onChange: () => void) => {
        dockviewMocks.listeners.add(onChange);
        return () => dockviewMocks.listeners.delete(onChange);
      },
      () => selector(dockviewMocks.state),
    ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      tasks: { activeTaskId: "task-1", activeSessionId: "session-1" },
      workspaces: { activeId: "workspace-1" },
      repositories: { itemsByWorkspaceId: { "workspace-1": [] } },
      removeTaskMR: vi.fn(),
      setMobileSessionReview: vi.fn(),
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

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("./task-mr-link-dialog", () => ({ TaskMRLinkDialog: () => null }));
vi.mock("./mr-automation-controls", () => ({ MRAutomationControls: () => null }));
vi.mock("./mr-ci-popover", () => ({ MRCIPopover: () => null }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

function singleMR(): TaskMR {
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
  } as TaskMR;
}

describe("MRTopbarButton desktop review opener — layout settling", () => {
  beforeEach(() => {
    gitlabMocks.mrs = [singleMR()];
    dockviewMocks.addMRPanel.mockClear();
    dockviewMocks.set({ api: {}, currentLayoutEnvId: null, isRestoringLayout: false });
  });

  afterEach(cleanup);

  it("re-opens the panel after the env-switch restore that would have discarded it", () => {
    // Session still loading: dockview api exists, but no env has been adopted.
    render(createElement(MRTopbarButton));
    act(() => {
      fireEvent.click(screen.getByTestId(TRIGGER_TESTID));
    });
    // Opened immediately so the click gives instant feedback...
    expect(dockviewMocks.addMRPanel).toHaveBeenCalled();
    const callsBeforeRestore = dockviewMocks.addMRPanel.mock.calls.length;

    // ...then the env hydrates and switchEnvLayout replaces the layout,
    // discarding that panel. Once it settles, the open must be re-applied.
    act(() => {
      dockviewMocks.set({ currentLayoutEnvId: "env-1", isRestoringLayout: false });
    });

    expect(dockviewMocks.addMRPanel.mock.calls.length).toBeGreaterThan(callsBeforeRestore);
  });

  it("does not re-open the panel on a later layout change once the env has settled", () => {
    // Env already adopted at click time — the common case for an established
    // session. The intent must not be retained, or a later preset switch or
    // maximize would resurrect a panel the user has since closed.
    dockviewMocks.set({ currentLayoutEnvId: "env-1" });
    render(createElement(MRTopbarButton));
    act(() => {
      fireEvent.click(screen.getByTestId(TRIGGER_TESTID));
    });
    const callsAfterClick = dockviewMocks.addMRPanel.mock.calls.length;
    expect(callsAfterClick).toBeGreaterThan(0);

    // A later user-initiated layout restore (preset switch / maximize).
    act(() => {
      dockviewMocks.set({ isRestoringLayout: true });
    });
    act(() => {
      dockviewMocks.set({ isRestoringLayout: false });
    });

    expect(dockviewMocks.addMRPanel.mock.calls.length).toBe(callsAfterClick);
  });

  it("retains the intent when a stale env id makes the layout look settled mid-remount", () => {
    // `setApi(null)` does not reset `currentLayoutEnvId`, so navigating between
    // tasks leaves the previous env id in place while the new dockview mounts.
    // "Settled" must not be trusted on its own — without the dockviewReady
    // check the click would be dropped and no later API init could open it.
    dockviewMocks.set({ api: null, currentLayoutEnvId: "stale-env", isRestoringLayout: false });
    render(createElement(MRTopbarButton));
    act(() => {
      fireEvent.click(screen.getByTestId(TRIGGER_TESTID));
    });
    expect(dockviewMocks.addMRPanel).not.toHaveBeenCalled();

    act(() => {
      dockviewMocks.set({ api: {}, currentLayoutEnvId: "env-1" });
    });
    expect(dockviewMocks.addMRPanel).toHaveBeenCalled();
  });

  it("defers the open until dockview exists when clicked before onReady", () => {
    dockviewMocks.set({ api: null });
    render(createElement(MRTopbarButton));
    act(() => {
      fireEvent.click(screen.getByTestId(TRIGGER_TESTID));
    });
    expect(dockviewMocks.addMRPanel).not.toHaveBeenCalled();

    act(() => {
      dockviewMocks.set({ api: {}, currentLayoutEnvId: "env-1" });
    });
    expect(dockviewMocks.addMRPanel).toHaveBeenCalled();
  });
});
