import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { PRStatusChip } from "./pr-status-chip";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const isMobileMock = vi.fn(() => false);
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => isMobileMock(),
}));

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/github-api")>();
  return {
    ...actual,
    getPRFeedback: vi.fn().mockResolvedValue(null),
    listWorkspaceTaskPRs: vi.fn().mockResolvedValue({ task_prs: {} }),
  };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: vi.fn(() => null),
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

beforeEach(() => {
  isMobileMock.mockReturnValue(false);
});

afterEach(() => {
  cleanup();
  isMobileMock.mockReset();
});

const CHIP_TESTID = "pr-status-chip";
const seededState: Partial<AppState> = {
  taskPRs: { byTaskId: { "task-1": [makePR()] } },
};

describe("PRStatusChip", () => {
  it("returns null when the task has no PR", () => {
    renderWithStore(undefined, <PRStatusChip taskId="missing" />);
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });

  describe("desktop branch", () => {
    beforeEach(() => isMobileMock.mockReturnValue(false));

    it("renders the chip button without a Drawer", () => {
      renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
      const chip = screen.getByTestId(CHIP_TESTID);
      expect(chip).toBeTruthy();
      // The chip's HoverCard popover is hover-only on desktop; clicking the
      // chip must not surface the mobile Drawer testid.
      act(() => {
        fireEvent.click(chip);
      });
      expect(document.querySelector("[data-testid='pr-status-chip-drawer']")).toBeNull();
    });

    it("exposes the canonical data attributes that desktop tests rely on", () => {
      renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
      const chip = screen.getByTestId(CHIP_TESTID);
      expect(chip.getAttribute("data-pr-number")).toBe("42");
      expect(chip.getAttribute("data-pr-state")).toBe("open");
      expect(chip.getAttribute("data-status")).toBe("passed");
      expect(chip.getAttribute("data-pr-ready-to-merge")).toBe("true");
    });
  });

  describe("mobile branch", () => {
    beforeEach(() => isMobileMock.mockReturnValue(true));

    it("renders the chip closed and opens the drawer on click", () => {
      renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
      // Drawer must not be in the DOM before the user taps the chip — relied
      // on by the e2e spec's `toHaveCount(0)` precondition.
      expect(document.querySelector("[data-testid='pr-status-chip-drawer']")).toBeNull();

      const chip = screen.getByTestId(CHIP_TESTID);
      act(() => {
        fireEvent.click(chip);
      });

      const drawer = document.querySelector("[data-testid='pr-status-chip-drawer']");
      expect(drawer).not.toBeNull();
      // Inner popover body + close button render inside the drawer.
      expect(document.querySelector("[data-testid='pr-topbar-popover-inner']")).not.toBeNull();
      expect(document.querySelector("[data-testid='pr-status-chip-drawer-close']")).not.toBeNull();
    });

    it("preserves the same data attributes as the desktop chip", () => {
      renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
      const chip = screen.getByTestId(CHIP_TESTID);
      expect(chip.getAttribute("data-pr-number")).toBe("42");
      expect(chip.getAttribute("data-pr-state")).toBe("open");
      expect(chip.getAttribute("data-status")).toBe("passed");
      expect(chip.getAttribute("data-pr-ready-to-merge")).toBe("true");
    });

    it("reflects a failed PR with data-status='failed'", () => {
      renderWithStore(
        { taskPRs: { byTaskId: { "task-1": [makePR({ checks_state: "failure" })] } } },
        <PRStatusChip taskId="task-1" />,
      );
      expect(screen.getByTestId(CHIP_TESTID).getAttribute("data-status")).toBe("failed");
    });

    // NOTE: vaul's close animation depends on CSS transition events that
    // happy-dom does not fire, so the drawer never unmounts in this env.
    // The mobile-pr-ci-chip.spec.ts e2e covers close-button dismissal in a
    // real browser.

    it("renders the no-checks empty state in the drawer when the PR has no checks", () => {
      renderWithStore(
        {
          taskPRs: {
            byTaskId: {
              "task-1": [
                makePR({
                  checks_state: "",
                  checks_total: 0,
                  checks_passing: 0,
                  review_state: "",
                  mergeable_state: "",
                }),
              ],
            },
          },
        },
        <PRStatusChip taskId="task-1" />,
      );
      act(() => {
        fireEvent.click(screen.getByTestId(CHIP_TESTID));
      });
      expect(document.querySelector("[data-testid='pr-checks-empty']")).not.toBeNull();
    });
  });
});
