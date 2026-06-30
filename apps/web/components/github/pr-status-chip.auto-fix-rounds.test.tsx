import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { PRStatusChip } from "./pr-status-chip";
import type { AppState } from "@/lib/state/store";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

const AUTO_FIX_BADGE_TESTID = "pr-status-auto-fix-chip";
const CHIP_TESTID = "pr-status-chip";
const ROUND_HELP_TESTID = "ci-auto-fix-round-help";
const ROUND_EXPLANATION_TESTID = "ci-auto-fix-round-explanation";

const testConstants = vi.hoisted(() => ({
  defaultCIFixPrompt: "Default CI fix prompt",
}));

const responsiveMock = vi.hoisted(() => ({
  breakpoint: "desktop" as "mobile" | "tablet" | "compactDesktop" | "desktop",
  isFinePointer: true,
}));

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
    getTaskCIAutomationOptions: vi.fn().mockResolvedValue(makeCIOptions()),
    listWorkspaceTaskPRs: vi.fn().mockResolvedValue({ task_prs: {} }),
  };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: vi.fn(() => null),
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
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 1,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 2,
    checks_passing: 2,
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

function makeCIOptions(overrides: Partial<TaskCIAutomationOptions> = {}): TaskCIAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: testConstants.defaultCIFixPrompt,
    using_default_prompt: true,
    updated_at: "2026-06-18T10:00:00Z",
    auto_fix_max_rounds: 10,
    pr_states: [],
    ...overrides,
  };
}

function makeCIPrState(roundCount: number, exhausted = false) {
  return {
    task_id: "task-1",
    repository_id: "",
    pr_number: 42,
    last_fix_signature: "",
    last_fix_checkpoint_json: "",
    last_fix_enqueued_at: null,
    last_fix_session_id: null,
    auto_fix_round_count: roundCount,
    auto_fix_exhausted_at: exhausted ? "2026-06-18T11:00:00Z" : null,
    last_merge_signature: "",
    last_merge_attempt_at: null,
    last_error: exhausted ? "CI auto-fix paused after 10 rounds for this PR" : null,
    created_at: "2026-06-18T10:00:00Z",
    updated_at: "2026-06-18T10:00:00Z",
  };
}

function stateWithAutoFix(
  roundCount: number,
  exhausted = false,
  maxRounds = 10,
): Partial<AppState> {
  return {
    taskPRs: { byTaskId: { "task-1": [makePR()] } },
    taskCIAutomation: {
      byTaskId: {
        "task-1": makeCIOptions({
          auto_fix_enabled: true,
          auto_fix_max_rounds: maxRounds,
          pr_states: [makeCIPrState(roundCount, exhausted)],
        }),
      },
      loading: {},
      saving: {},
      errors: {},
    },
  };
}

beforeEach(() => {
  responsiveMock.breakpoint = "desktop";
  responsiveMock.isFinePointer = true;
});

afterEach(() => {
  cleanup();
});

describe("PRStatusChip auto-fix round display", () => {
  it("shows the CI auto-fix round count in the enabled chip", () => {
    renderWithStore(stateWithAutoFix(3), <PRStatusChip taskId="task-1" />);

    expect(screen.getByTestId(AUTO_FIX_BADGE_TESTID).textContent).toBe("Auto-fix 3/10");
  });

  it("shows exhausted auto-fix state as a warning chip", () => {
    renderWithStore(stateWithAutoFix(10, true), <PRStatusChip taskId="task-1" />);

    const chip = screen.getByTestId(AUTO_FIX_BADGE_TESTID);
    expect(chip.textContent).toBe("Auto-fix 10/10");
    expect(chip.getAttribute("data-auto-fix-exhausted")).toBe("true");
  });

  it("does not mark 10/10 as exhausted until the backend pauses auto-fix", () => {
    renderWithStore(stateWithAutoFix(10), <PRStatusChip taskId="task-1" />);

    const chip = screen.getByTestId(AUTO_FIX_BADGE_TESTID);
    expect(chip.textContent).toBe("Auto-fix 10/10");
    expect(chip.getAttribute("data-auto-fix-exhausted")).toBe("false");
  });

  it("uses the max round count returned by the backend", () => {
    renderWithStore(stateWithAutoFix(3, false, 12), <PRStatusChip taskId="task-1" />);

    expect(screen.getByTestId(AUTO_FIX_BADGE_TESTID).textContent).toBe("Auto-fix 3/12");
  });

  it("explains the auto-fix round count only after hovering the round help icon", async () => {
    renderWithStore(stateWithAutoFix(1), <PRStatusChip taskId="task-1" />);

    fireEvent.mouseEnter(screen.getByTestId(CHIP_TESTID));
    expect(screen.queryByTestId(ROUND_EXPLANATION_TESTID)).toBeNull();

    const roundHelp = await screen.findByTestId(ROUND_HELP_TESTID);
    fireEvent.pointerMove(roundHelp, { pointerType: "mouse" });
    fireEvent.pointerEnter(roundHelp, { pointerType: "mouse" });
    fireEvent.mouseMove(roundHelp);
    const [explanation] = await screen.findAllByTestId(ROUND_EXPLANATION_TESTID);
    expect(explanation.textContent).toContain("Auto-fix has used 1 of 10 rounds");
    expect(explanation.textContent).toContain(
      "Kandev waits for all PR checks to finish before starting a new CI auto-fix turn",
    );
    expect(explanation.textContent).toContain(
      "Updating an already queued auto-fix message does not use another round",
    );
    expect(explanation.textContent).toContain(
      "pauses auto-fix for this PR so it cannot loop forever",
    );
  });

  it("shows the auto-fix round help icon in the mobile drawer", () => {
    responsiveMock.breakpoint = "mobile";
    responsiveMock.isFinePointer = false;
    renderWithStore(stateWithAutoFix(2), <PRStatusChip taskId="task-1" />);

    act(() => {
      fireEvent.click(screen.getByTestId(CHIP_TESTID));
    });

    expect(screen.getByTestId(ROUND_HELP_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(ROUND_EXPLANATION_TESTID)).toBeNull();
  });
});
