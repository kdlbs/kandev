import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { PRStatusChip, aggregateChipStatus } from "./pr-status-chip";
import { PR_CI_DESKTOP_POPOVER_SCROLL_CLASS } from "./pr-ci-popover";
import type { AppState } from "@/lib/state/store";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

const isMobileMock = vi.fn(() => false);
vi.mock("@/hooks/use-mobile", () => ({
  useIsMobile: () => isMobileMock(),
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

function makeCIOptions(overrides: Partial<TaskCIAutomationOptions> = {}): TaskCIAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "Default CI fix prompt",
    using_default_prompt: true,
    updated_at: "2026-06-18T10:00:00Z",
    pr_states: [],
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
const ATTR_PR_NUMBER = "data-pr-number";
const ATTR_STATUS = "data-status";
const DRAWER_SELECTOR = "[data-testid='pr-status-chip-drawer']";
const seededState: Partial<AppState> = {
  taskPRs: { byTaskId: { "task-1": [makePR()] } },
  taskCIAutomation: {
    byTaskId: { "task-1": makeCIOptions() },
    loading: {},
    saving: {},
    errors: {},
  },
};

function multiState(prs: TaskPR[]): Partial<AppState> {
  return { taskPRs: { byTaskId: { "task-1": prs } } };
}

async function expectDesktopHoverPopoverConstrained() {
  fireEvent.pointerEnter(screen.getByTestId(CHIP_TESTID));
  const inner = await screen.findByTestId("pr-topbar-popover-inner");
  const content = inner.closest<HTMLElement>(".overflow-y-auto");
  expect(content).not.toBeNull();
  const classNames = content!.className.split(/\s+/);
  expect(classNames).toEqual(
    expect.arrayContaining(PR_CI_DESKTOP_POPOVER_SCROLL_CLASS.split(/\s+/)),
  );
}

describe("PRStatusChip", () => {
  it("returns null when the task has no PR", () => {
    renderWithStore(undefined, <PRStatusChip taskId="missing" />);
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });

  it("returns null when the PR has been merged (terminal state)", () => {
    renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [makePR({ state: "merged" })] } } },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });

  it("returns null when the PR has been closed (terminal state)", () => {
    renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [makePR({ state: "closed" })] } } },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });
});

describe("PRStatusChip desktop branch", () => {
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
    expect(document.querySelector(DRAWER_SELECTOR)).toBeNull();
  });

  it("constrains the hover popover to the available viewport height", async () => {
    renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
    await expectDesktopHoverPopoverConstrained();
  });

  it("exposes the canonical data attributes that desktop tests rely on", () => {
    renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
    const chip = screen.getByTestId(CHIP_TESTID);
    expect(chip.getAttribute(ATTR_PR_NUMBER)).toBe("42");
    expect(chip.getAttribute("data-pr-state")).toBe("open");
    expect(chip.getAttribute(ATTR_STATUS)).toBe("passed");
    expect(chip.getAttribute("data-pr-ready-to-merge")).toBe("true");
  });

  it("shows automation badges when auto-fix or auto-merge are enabled", () => {
    renderWithStore(
      {
        taskPRs: { byTaskId: { "task-1": [makePR()] } },
        taskCIAutomation: {
          byTaskId: {
            "task-1": makeCIOptions({ auto_fix_enabled: true, auto_merge_enabled: true }),
          },
          loading: {},
          saving: {},
          errors: {},
        },
      },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.getByTestId("pr-status-auto-fix-chip").textContent).toBe("Auto-fix");
    expect(screen.getByTestId("pr-status-auto-merge-chip").textContent).toBe("Auto-merge");
    expect(screen.getByTestId(CHIP_TESTID).getAttribute("aria-label")).toBe(
      "Pull request #42 CI status, auto-fix enabled, auto-merge enabled",
    );
  });
});

describe("PRStatusChip mobile branch", () => {
  beforeEach(() => isMobileMock.mockReturnValue(true));

  it("renders the chip closed and opens the drawer on click", () => {
    renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
    // Drawer must not be in the DOM before the user taps the chip — relied
    // on by the e2e spec's `toHaveCount(0)` precondition.
    expect(document.querySelector(DRAWER_SELECTOR)).toBeNull();

    const chip = screen.getByTestId(CHIP_TESTID);
    act(() => {
      fireEvent.click(chip);
    });

    const drawer = document.querySelector(DRAWER_SELECTOR);
    expect(drawer).not.toBeNull();
    // Inner popover body + close button render inside the drawer.
    expect(document.querySelector("[data-testid='pr-topbar-popover-inner']")).not.toBeNull();
    expect(document.querySelector("[data-testid='pr-status-chip-drawer-close']")).not.toBeNull();
    expect(screen.getByTestId("pr-popover-title").textContent).toBe("Test PR");
    expect(drawer?.textContent).not.toContain("Open PR details");
  });

  it("preserves the same data attributes as the desktop chip", () => {
    renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
    const chip = screen.getByTestId(CHIP_TESTID);
    expect(chip.getAttribute(ATTR_PR_NUMBER)).toBe("42");
    expect(chip.getAttribute("data-pr-state")).toBe("open");
    expect(chip.getAttribute(ATTR_STATUS)).toBe("passed");
    expect(chip.getAttribute("data-pr-ready-to-merge")).toBe("true");
  });

  it("reflects a failed PR with data-status='failed'", () => {
    renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [makePR({ checks_state: "failure" })] } } },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.getByTestId(CHIP_TESTID).getAttribute(ATTR_STATUS)).toBe("failed");
  });

  it("shows automation badges on the mobile chip trigger", () => {
    renderWithStore(
      {
        taskPRs: { byTaskId: { "task-1": [makePR()] } },
        taskCIAutomation: {
          byTaskId: { "task-1": makeCIOptions({ auto_fix_enabled: true }) },
          loading: {},
          saving: {},
          errors: {},
        },
      },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.getByTestId("pr-status-auto-fix-chip").textContent).toBe("Auto-fix");
    expect(screen.queryByTestId("pr-status-auto-merge-chip")).toBeNull();
    expect(screen.getByTestId(CHIP_TESTID).getAttribute("aria-label")).toBe(
      "Pull request #42 CI status, auto-fix enabled",
    );
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

describe("PRStatusChip — mergeability", () => {
  it("is 'conflict' (not 'passed') for a dirty PR even with green checks + approval", () => {
    // Regression: the chip read mergeable_state-blind and showed the green
    // "passed" check on a PR that actually had merge conflicts.
    renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [makePR({ mergeable_state: "dirty" })] } } },
      <PRStatusChip taskId="task-1" />,
    );
    const chip = screen.getByTestId(CHIP_TESTID);
    expect(chip.getAttribute(ATTR_STATUS)).toBe("conflict");
    expect(chip.getAttribute("data-pr-ready-to-merge")).toBe("false");
  });

  it("is 'behind' for a behind-base PR that is otherwise green", () => {
    renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [makePR({ mergeable_state: "behind" })] } } },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.getByTestId(CHIP_TESTID).getAttribute(ATTR_STATUS)).toBe("behind");
  });

  it("is 'waiting' for normal branch protection after checks pass", () => {
    renderWithStore(
      { taskPRs: { byTaskId: { "task-1": [makePR({ mergeable_state: "blocked" })] } } },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.getByTestId(CHIP_TESTID).getAttribute(ATTR_STATUS)).toBe("waiting");
  });

  it("stays 'in_progress' for a blocked PR that is still awaiting a requested review", () => {
    // Blocked because a reviewer is still pending → that's the awaiting-review
    // gate, not a generic protection block. Keep the in-progress reading.
    renderWithStore(
      {
        taskPRs: {
          byTaskId: {
            "task-1": [makePR({ mergeable_state: "blocked", pending_review_count: 1 })],
          },
        },
      },
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.getByTestId(CHIP_TESTID).getAttribute(ATTR_STATUS)).toBe("in_progress");
  });
});

describe("PRStatusChip CI automation mobile parity", () => {
  beforeEach(() => isMobileMock.mockReturnValue(true));

  it("renders controls and prompt editing inside the drawer", async () => {
    renderWithStore(seededState, <PRStatusChip taskId="task-1" />);
    act(() => {
      fireEvent.click(screen.getByTestId(CHIP_TESTID));
    });

    const drawer = document.querySelector(DRAWER_SELECTOR);
    expect(drawer?.textContent).toContain("Auto-fix CI and address comments");
    expect(drawer?.textContent).toContain("Auto-merge when ready");

    act(() => {
      fireEvent.click(screen.getByLabelText("Edit auto-fix prompt for this task"));
    });
    await waitFor(() => {
      expect(
        screen.getAllByRole("dialog").some((el) => el.textContent?.includes("Auto-fix prompt")),
      ).toBe(true);
    });
    expect(screen.getByRole("link", { name: "Edit default prompt" }).getAttribute("href")).toBe(
      "/settings/prompts",
    );
  });
});

describe("aggregateChipStatus", () => {
  it("returns 'neutral' for an empty list", () => {
    expect(aggregateChipStatus([])).toBe("neutral");
  });

  it("lets one failing PR dominate a passing sibling", () => {
    const passing = makePR();
    const failing = makePR({ id: "fail", checks_state: "failure" });
    expect(aggregateChipStatus([passing, failing])).toBe("failed");
  });

  it("returns 'in_progress' when the worst is a pending PR", () => {
    const passing = makePR();
    const pending = makePR({ id: "pend", review_state: "", checks_state: "pending" });
    expect(aggregateChipStatus([passing, pending])).toBe("in_progress");
  });

  it("lets a conflicting PR dominate a passing sibling", () => {
    const passing = makePR();
    const conflict = makePR({ id: "dirty", mergeable_state: "dirty" });
    expect(aggregateChipStatus([passing, conflict])).toBe("conflict");
  });

  it("ranks a failing PR above a conflicting one", () => {
    const conflict = makePR({ id: "dirty", mergeable_state: "dirty" });
    const failing = makePR({ id: "fail", checks_state: "failure" });
    expect(aggregateChipStatus([conflict, failing])).toBe("failed");
  });
});

describe("PRStatusChip — multiple PRs", () => {
  const TWO_OPEN = [
    makePR({ id: "a", pr_number: 1 }),
    makePR({ id: "b", pr_number: 2, checks_state: "failure" }),
  ];

  it("renders one aggregate chip with a PR count and worst-of status", () => {
    renderWithStore(multiState(TWO_OPEN), <PRStatusChip taskId="task-1" />);
    const chip = screen.getByTestId(CHIP_TESTID);
    expect(chip.getAttribute("data-pr-count")).toBe("2");
    // PR #2 is failing, so the aggregate glyph is red.
    expect(chip.getAttribute(ATTR_STATUS)).toBe("failed");
  });

  it("constrains the aggregate hover popover to the available viewport height", async () => {
    renderWithStore(multiState(TWO_OPEN), <PRStatusChip taskId="task-1" />);
    await expectDesktopHoverPopoverConstrained();
  });

  it("stays visible while at least one PR is still open", () => {
    renderWithStore(
      multiState([makePR({ id: "a", state: "merged" }), makePR({ id: "b", pr_number: 2 })]),
      <PRStatusChip taskId="task-1" />,
    );
    // Only one PR is open, so it renders as the single-PR chip (its number).
    expect(screen.getByTestId(CHIP_TESTID).getAttribute(ATTR_PR_NUMBER)).toBe("2");
  });

  it("returns null only when every PR is terminal", () => {
    renderWithStore(
      multiState([makePR({ id: "a", state: "merged" }), makePR({ id: "b", state: "closed" })]),
      <PRStatusChip taskId="task-1" />,
    );
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });

  describe("mobile drawer", () => {
    beforeEach(() => isMobileMock.mockReturnValue(true));

    it("opens a drawer with the tabbed multi-PR popover", () => {
      renderWithStore(multiState(TWO_OPEN), <PRStatusChip taskId="task-1" />);
      act(() => {
        fireEvent.click(screen.getByTestId(CHIP_TESTID));
      });
      expect(document.querySelector(DRAWER_SELECTOR)).not.toBeNull();
      expect(document.querySelector("[data-testid='pr-multi-popover']")).not.toBeNull();
      // One tab per PR (testid is repo-scoped so same-number PRs on
      // different repos stay unique).
      expect(document.querySelector("[data-testid='pr-popover-tab-demo-1']")).not.toBeNull();
      expect(document.querySelector("[data-testid='pr-popover-tab-demo-2']")).not.toBeNull();
    });
  });
});
