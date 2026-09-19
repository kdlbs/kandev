import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { getPRFeedback } from "@/lib/api/domains/github-api";
import { PRStatusChip } from "./pr-status-chip";
import { PRTopbarButton } from "./pr-topbar-button";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const responsiveMock = vi.hoisted(() => ({
  breakpoint: "desktop" as "mobile" | "tablet" | "compactDesktop" | "desktop",
  isFinePointer: true,
}));
const wsMock = vi.hoisted(() => ({
  client: null as { request: ReturnType<typeof vi.fn> } | null,
}));
const CHIP_TESTID = "pr-status-chip";
const TOPBAR_BUTTON_TESTID = "pr-topbar-button";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: responsiveMock.breakpoint,
    isMobile: responsiveMock.breakpoint === "mobile",
    isTablet: responsiveMock.breakpoint === "tablet",
    isDesktop:
      responsiveMock.breakpoint === "compactDesktop" || responsiveMock.breakpoint === "desktop",
    isCompactDesktop: responsiveMock.breakpoint === "compactDesktop",
    isFullDesktop: responsiveMock.breakpoint === "desktop",
    isFinePointer: responsiveMock.isFinePointer,
    usesDesktopWorkbench:
      responsiveMock.breakpoint === "compactDesktop" || responsiveMock.breakpoint === "desktop",
  }),
}));

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/github-api")>();
  return {
    ...actual,
    getPRFeedback: vi.fn().mockResolvedValue(null),
    getTaskCIAutomationOptions: vi.fn().mockResolvedValue({
      task_id: "task-1",
      auto_fix_enabled: false,
      auto_merge_enabled: false,
      auto_fix_prompt_override: null,
      effective_auto_fix_prompt: "Default CI fix prompt",
      using_default_prompt: true,
      updated_at: "2026-06-18T10:00:00Z",
      pr_states: [],
    }),
    listWorkspaceTaskPRs: vi.fn().mockResolvedValue({ task_prs: {} }),
  };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => wsMock.client,
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { addPRPanel: () => void }) => unknown) =>
    selector({ addPRPanel: vi.fn() }),
}));

function renderWithStore(initialState: Partial<AppState>, ui: ReactNode) {
  return render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <TooltipProvider>{ui}</TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-id",
    workspace_id: "workspace-1",
    task_id: "task-1",
    owner: "acme",
    repo: "demo",
    pr_number: 42,
    pr_url: "https://github.com/acme/demo/pull/42",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "failure",
    mergeable_state: "blocked",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 2,
    checks_passing: 1,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

function taskState(prs: TaskPR[], activeTask = false): Partial<AppState> {
  return {
    workspaces: { items: [], activeId: "ws-1" },
    taskPRs: { byTaskId: { "task-1": prs } },
    ...(activeTask
      ? {
          tasks: {
            activeTaskId: "task-1",
            activeSessionId: null,
            pinnedSessionId: null,
            lastSessionByTaskId: {},
            resumeSkippedSessionIds: {},
          },
        }
      : {}),
  };
}

function setupRefresh(prs: TaskPR[]) {
  const pending = prs.map((pr) => ({ ...pr, checks_state: "pending" as const }));
  const request = vi.fn().mockResolvedValueOnce({ prs }).mockResolvedValueOnce({ prs: pending });
  wsMock.client = { request };
  return request;
}

async function expectInitialFailed(request: ReturnType<typeof vi.fn>, testId: string) {
  await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  expect(screen.getByTestId(testId).getAttribute("data-status")).toBe("failed");
}

async function expectRefresh(request: ReturnType<typeof vi.fn>, testId: string) {
  await waitFor(() =>
    expect(screen.getByTestId(testId).getAttribute("data-status")).toBe("in_progress"),
  );
  await waitFor(() => expect(request).toHaveBeenCalledTimes(2));
  await expectFeedbackRefresh();
}

function expectTopbarStatusIcon(colorClass: string) {
  expect(
    screen.getByTestId(TOPBAR_BUTTON_TESTID).querySelector(`svg.${colorClass}`),
  ).not.toBeNull();
}

async function expectFeedbackRefresh() {
  await waitFor(() =>
    expect(getPRFeedback).toHaveBeenCalledWith("ws-1", "acme", "demo", 42, {
      cache: "no-store",
    }),
  );
}

async function expectTopbarRefresh(request: ReturnType<typeof vi.fn>) {
  await waitFor(() => expect(request).toHaveBeenCalledTimes(2));
  await expectFeedbackRefresh();
  await waitFor(() => expectTopbarStatusIcon("text-yellow-500"));
}

beforeEach(() => {
  responsiveMock.breakpoint = "desktop";
  responsiveMock.isFinePointer = true;
  wsMock.client = null;
  vi.mocked(getPRFeedback).mockClear();
});

afterEach(() => {
  cleanup();
});

describe("TaskPR refresh routes", () => {
  it("refreshes the stale single-PR chip when its mobile drawer opens", async () => {
    responsiveMock.breakpoint = "mobile";
    responsiveMock.isFinePointer = false;
    const failed = makePR();
    const request = setupRefresh([failed]);

    renderWithStore(taskState([failed]), <PRStatusChip taskId="task-1" />);
    await expectInitialFailed(request, CHIP_TESTID);
    fireEvent.click(screen.getByTestId(CHIP_TESTID));

    await expectRefresh(request, CHIP_TESTID);
  });

  it("refreshes stale aggregate CI when the multi-PR chip hover popover opens", async () => {
    const failed = makePR({ id: "failed" });
    const passing = makePR({
      id: "passing",
      pr_number: 43,
      checks_state: "success",
      checks_passing: 2,
    });
    const request = setupRefresh([failed, passing]);

    renderWithStore(taskState([failed, passing]), <PRStatusChip taskId="task-1" />);
    await expectInitialFailed(request, CHIP_TESTID);
    fireEvent.mouseEnter(screen.getByTestId(CHIP_TESTID));

    await expectRefresh(request, CHIP_TESTID);
  });

  it("refreshes stale aggregate CI when the multi-PR chip mobile drawer opens", async () => {
    responsiveMock.breakpoint = "mobile";
    responsiveMock.isFinePointer = false;
    const failed = makePR({ id: "failed" });
    const passing = makePR({
      id: "passing",
      pr_number: 43,
      checks_state: "success",
      checks_passing: 2,
    });
    const request = setupRefresh([failed, passing]);

    renderWithStore(taskState([failed, passing]), <PRStatusChip taskId="task-1" />);
    await expectInitialFailed(request, CHIP_TESTID);
    fireEvent.click(screen.getByTestId(CHIP_TESTID));

    await expectRefresh(request, CHIP_TESTID);
  });

  it("refreshes a stale single-PR topbar status when its desktop popover opens", async () => {
    const failed = makePR();
    const request = setupRefresh([failed]);

    renderWithStore(taskState([failed], true), <PRTopbarButton />);
    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expectTopbarStatusIcon("text-red-500");
    fireEvent.mouseEnter(screen.getByTestId(TOPBAR_BUTTON_TESTID));

    await expectTopbarRefresh(request);
  });

  it("refreshes stale aggregate CI when the multi-PR topbar popover opens", async () => {
    const failed = makePR({ id: "failed" });
    const passing = makePR({
      id: "passing",
      pr_number: 43,
      checks_state: "success",
      checks_passing: 2,
    });
    const request = setupRefresh([failed, passing]);

    renderWithStore(taskState([failed, passing], true), <PRTopbarButton />);
    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expectTopbarStatusIcon("text-red-500");
    fireEvent.mouseEnter(screen.getByTestId(TOPBAR_BUTTON_TESTID));

    await expectTopbarRefresh(request);
  });
});
