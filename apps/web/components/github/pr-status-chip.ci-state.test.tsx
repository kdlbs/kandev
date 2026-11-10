import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { PRStatusChip } from "./pr-status-chip";
import { makeTestPR as makePR } from "./pr-status-chip.test-fixtures";
import type { AppState } from "@/lib/state/store";

const CHIP_TESTID = "pr-status-chip";
const ATTR_STATUS = "data-status";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: "desktop",
    isMobile: false,
    isTablet: false,
    isDesktop: true,
    isCompactDesktop: false,
    isFullDesktop: true,
    isFinePointer: true,
    usesDesktopWorkbench: true,
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

function renderWithStore(initialState: Partial<AppState> | undefined, ui: ReactNode) {
  return render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <TooltipProvider>{ui}</TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

beforeEach(() => undefined);

afterEach(() => cleanup());

describe("PRStatusChip draft CI bubble", () => {
  it("keeps a failed draft PR on the existing failure composition without a second bubble", () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            "task-1": [makePR({ mergeable_state: "draft", checks_state: "failure" })],
          },
        },
      },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.getByTestId(CHIP_TESTID).getAttribute(ATTR_STATUS)).toBe("failed");
    expect(screen.queryByTestId("pr-ci-bubble-failed")).toBeNull();
  });

  it("renders the shared draft PR glyph instead of a muted dot", () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: { "task-1": [makePR({ mergeable_state: "draft", checks_state: "success" })] },
        },
      },
      <PRStatusChip taskId="task-1" />,
    );
    const chip = screen.getByTestId(CHIP_TESTID);
    expect(chip.getAttribute(ATTR_STATUS)).toBe("draft");
    expect(chip.querySelector(".tabler-icon-git-pull-request")).not.toBeNull();
    expect(screen.queryByTestId("pr-ci-bubble-failed")).toBeNull();
  });

  it("keeps the conflict triangle beside the shared draft glyph", () => {
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            "task-1": [
              makePR({
                mergeable_state: "draft",
                checks_state: "success",
                has_merge_conflicts: true,
              }),
            ],
          },
        },
      },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.getByTestId(CHIP_TESTID).getAttribute(ATTR_STATUS)).toBe("draft");
    expect(screen.getByTestId("pr-merge-conflict-warning")).not.toBeNull();
  });
});
