import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { PRStatusChip, aggregateChipStatus } from "./pr-status-chip";
import { makeTestPR as makePR } from "./pr-status-chip.test-fixtures";
import type { AppState } from "@/lib/state/store";
import type { TaskPR, WorkflowAttention } from "@/lib/types/github";

const CHIP_TESTID = "pr-status-chip";

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
    getTaskCIAutomationOptions: vi.fn().mockResolvedValue({
      task_id: "task-1",
      auto_fix_enabled: false,
      auto_merge_enabled: false,
      auto_fix_prompt_override: null,
      effective_auto_fix_prompt: "Default CI fix prompt",
      using_default_prompt: true,
      updated_at: "2026-06-18T10:00:00Z",
      pr_states: [],
      pr_options: [],
    }),
    listWorkspaceTaskPRs: vi.fn().mockResolvedValue({ task_prs: {} }),
  };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
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

function makeAttention(
  state: WorkflowAttention["state"],
  runs: WorkflowAttention["runs"] = [],
): WorkflowAttention {
  return {
    state,
    head_sha: "head-1",
    observed_at: "2026-09-10T10:00:00Z",
    stale: true,
    runs,
  };
}

function makeAttentionPR(attention: WorkflowAttention): TaskPR {
  return makePR({ head_sha: "head-1", workflow_attention: attention });
}

beforeEach(() => {
  responsiveMock.breakpoint = "desktop";
  responsiveMock.isFinePointer = true;
});

afterEach(() => {
  cleanup();
});

describe("workflow attention chip status", () => {
  it("marks current-head workflow attention as a warning", () => {
    expect(aggregateChipStatus([makeAttentionPR(makeAttention("approval_required"))])).toBe(
      "attention",
    );
  });
});

describe("workflow attention desktop popover", () => {
  it("shows the workflow reason and an external workflow link", async () => {
    const pr = makeAttentionPR(
      makeAttention("approval_required", [
        {
          run_id: 7,
          run_attempt: 1,
          workflow_id: 9,
          name: "Run tests",
          url: "https://github.com/acme/demo/actions/runs/7",
          reason: "approval_required",
        },
      ]),
    );
    renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [pr] } } },
      <PRStatusChip taskId="task-1" />,
    );

    fireEvent.mouseEnter(screen.getByTestId(CHIP_TESTID));
    const notice = await screen.findByTestId("pr-workflow-attention");
    expect(notice.textContent).toContain("Awaiting maintainer approval");
    expect(notice.textContent).toContain("Run tests");
    expect(screen.getByTestId("pr-workflow-attention-link").getAttribute("href")).toBe(
      "https://github.com/acme/demo/actions/runs/7",
    );
    expect(screen.getByTestId("pr-workflow-attention-link").className).toContain("min-h-11");
    expect(notice.textContent).toContain("Last known status");
  });

  it("shows unavailable workflow evidence without claiming approval", async () => {
    renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [makeAttentionPR(makeAttention("unknown"))] } } },
      <PRStatusChip taskId="task-1" />,
    );

    fireEvent.mouseEnter(screen.getByTestId(CHIP_TESTID));
    const notice = await screen.findByTestId("pr-workflow-attention");
    expect(notice.textContent).toContain("Workflow status unavailable");
    expect(notice.textContent).not.toContain("Awaiting maintainer approval");
  });
});
