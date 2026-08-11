import { createElement } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useHoverPopover } from "@/hooks/domains/github/use-hover-popover";
import { MRStatusChip } from "./mr-status-chip";
import type { TaskMR, TaskMRAutomationOptions } from "@/lib/types/gitlab";

const OPEN_DELAY_MS = 150;
const CHIP_TESTID = "mr-status-chip";
const DATA_MR_IID = "data-mr-iid";
const POPOVER_STUB_TESTID = "mr-ci-popover-stub";

const gitlabMocks = vi.hoisted(() => ({ mrs: [] as TaskMR[] }));
const touchMocks = vi.hoisted(() => ({ usesTouchDrawer: false }));
const automationMocks = vi.hoisted(() => ({
  options: null as TaskMRAutomationOptions | null,
}));
const dialogMocks = vi.hoisted(() => ({ lastProps: null as { open: boolean } | null }));
const unlinkMock = vi.hoisted(() => vi.fn(async () => {}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: "workspace-1" },
      repositories: { itemsByWorkspaceId: { "workspace-1": [] } },
    }),
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: () => true,
  useTaskMRs: () => gitlabMocks.mrs,
  useUnlinkTaskMR: () => unlinkMock,
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr-automation", () => ({
  useTaskMRAutomationOptions: () => ({ options: automationMocks.options }),
}));

vi.mock("@/hooks/domains/kanban/use-task-by-id", () => ({
  useTaskById: () => ({ id: "task-1", repositories: [] }),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchMocks.usesTouchDrawer,
}));

vi.mock("./task-mr-link-dialog", () => ({
  TaskMRLinkDialog: (props: { open: boolean }) => {
    dialogMocks.lastProps = props;
    return <div data-testid="mr-link-dialog-stub" data-open={props.open ? "true" : "false"} />;
  },
}));

vi.mock("./mr-ci-popover", () => ({
  MRCIPopover: ({
    mr,
    onLink,
    onUnlink,
  }: {
    mr: TaskMR;
    onLink: () => void;
    onUnlink: () => void;
  }) => (
    <div data-testid={POPOVER_STUB_TESTID}>
      popover for !{mr.mr_iid}
      <button type="button" onClick={onLink}>
        link another
      </button>
      <button type="button" onClick={onUnlink}>
        unlink
      </button>
    </div>
  ),
}));

vi.mock("./mr-topbar-button", () => ({
  useMRPopoverInteractions: () => {
    const hover = useHoverPopover({ openDelayMs: OPEN_DELAY_MS, closeDelayMs: OPEN_DELAY_MS });
    return { usesTouchDrawer: false, ...hover };
  },
}));

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
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
    state: "open",
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
  };
}

function makeAutomation(overrides: Partial<TaskMRAutomationOptions> = {}): TaskMRAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_max_rounds: 10,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    review_reviewer_username: "",
    updated_at: "",
    mr_states: [],
    ...overrides,
  };
}

function resetChipMocks() {
  vi.useFakeTimers();
  touchMocks.usesTouchDrawer = false;
  automationMocks.options = null;
  dialogMocks.lastProps = null;
  unlinkMock.mockClear();
}

function teardownChipMocks() {
  cleanup();
  vi.useRealTimers();
}

describe("MRStatusChip rendering and selection", () => {
  beforeEach(resetChipMocks);
  afterEach(teardownChipMocks);

  it("renders nothing when the task has no linked MRs", () => {
    gitlabMocks.mrs = [];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });

  it("renders nothing when the only linked MR is terminal", () => {
    gitlabMocks.mrs = [makeMR({ state: "merged" })];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    expect(screen.queryByTestId(CHIP_TESTID)).toBeNull();
  });

  it("renders the trigger for a single open MR with the expected attributes", () => {
    gitlabMocks.mrs = [makeMR({ pipeline_state: "failure" })];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);
    expect(trigger.getAttribute("data-status")).toBe("failed");
    expect(trigger.getAttribute("data-mr-count")).toBe("1");
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("81");
    expect(trigger.getAttribute("data-mr-state")).toBe("open");
    expect(trigger.getAttribute("data-mr-ready-to-merge")).toBe("false");
    expect(trigger.getAttribute("data-selection-frozen")).toBe("false");
  });

  it("selects the highest-ranked open MR and reports the total open count", () => {
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }),
      makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }),
      makeMR({ id: "c", mr_iid: 3, state: "merged" }),
    ];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);
    expect(trigger.getAttribute("data-status")).toBe("failed");
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("2");
    // Terminal MR is not counted toward data-mr-count.
    expect(trigger.getAttribute("data-mr-count")).toBe("2");
  });

  it("renders the auto-fix badge at round 0 with no lifecycle state, and the auto-merge badge", () => {
    gitlabMocks.mrs = [makeMR()];
    automationMocks.options = makeAutomation({
      auto_fix_enabled: true,
      auto_fix_max_rounds: 5,
      auto_merge_enabled: true,
    });
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    const badge = screen.getByTestId("mr-status-auto-fix-chip");
    expect(badge.textContent).toContain("0/5");
    expect(badge.getAttribute("data-auto-fix-exhausted")).toBe("false");
    expect(screen.getByTestId("mr-status-auto-merge-chip")).toBeTruthy();
  });

  it("renders neither badge when automation is disabled", () => {
    gitlabMocks.mrs = [makeMR()];
    automationMocks.options = makeAutomation();
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    expect(screen.queryByTestId("mr-status-auto-fix-chip")).toBeNull();
    expect(screen.queryByTestId("mr-status-auto-merge-chip")).toBeNull();
  });
});

describe("MRStatusChip disclosure interactions", () => {
  beforeEach(resetChipMocks);
  afterEach(teardownChipMocks);

  it("opens the hover popover on the fine-pointer variant and freezes the acted-on MR while open", () => {
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }),
      makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }),
    ];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);

    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });

    expect(screen.getByTestId(POPOVER_STUB_TESTID).textContent).toContain("!2");
    expect(trigger.getAttribute("data-selection-frozen")).toBe("true");
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("2");

    // Store update makes MR "a" the new live selection while the popover
    // stays open on "b": data-status tracks the live selection, data-mr-iid
    // stays frozen (spec: Selection - freezing while the disclosure is open).
    act(() => {
      gitlabMocks.mrs = [
        makeMR({ id: "a", mr_iid: 1, pipeline_state: "failure" }),
        makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }),
      ];
    });
  });

  it("keeps the aria-label naming the acted-on MR's own status while frozen, not the live selection's", () => {
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }), // running, rank 4
      makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }), // failed, rank 5
    ];
    const { rerender } = render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);

    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });

    // Frozen on MR "b" (failed) — live and acted-on agree here, a sanity check.
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("2");
    expect(trigger.getAttribute("aria-label")).toContain("pipeline failed");

    // MR "b" (the frozen, acted-on MR) becomes ready; MR "a" is unchanged and
    // now outranks it, so the LIVE selection flips to "a" (still running).
    // data-mr-iid stays on "b" (still open, so the freeze holds), and
    // data-status tracks the live selection ("running") by design — but the
    // aria-label describes the acted-on MR, so it must say "ready to merge"
    // (b's own status), never "pipeline running" (spec: Accessibility, "does
    // NOT follow data-status's live tracking").
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }),
      makeMR({ id: "b", mr_iid: 2, approval_state: "approved", pipeline_state: "success" }),
    ];
    act(() => {
      rerender(createElement(MRStatusChip, { taskId: "task-1" }));
    });

    expect(trigger.getAttribute(DATA_MR_IID)).toBe("2");
    expect(trigger.getAttribute("data-status")).toBe("running");
    expect(trigger.getAttribute("aria-label")).toContain("ready to merge");
    expect(trigger.getAttribute("aria-label")).not.toContain("pipeline running");
  });

  it("calls the unlink hook with the acted-on MR's association id", () => {
    gitlabMocks.mrs = [makeMR({ id: "assoc-xyz" })];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);
    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });
    fireEvent.click(screen.getByRole("button", { name: "unlink" }));
    expect(unlinkMock).toHaveBeenCalledWith("assoc-xyz");
  });

  it("closes the popover before opening the link dialog", () => {
    gitlabMocks.mrs = [makeMR()];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);
    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });
    expect(screen.getByTestId(POPOVER_STUB_TESTID)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "link another" }));

    expect(screen.queryByTestId(POPOVER_STUB_TESTID)).toBeNull();
    expect(dialogMocks.lastProps?.open).toBe(true);
  });
});

describe("MRStatusChip responsive variant", () => {
  beforeEach(resetChipMocks);
  afterEach(teardownChipMocks);

  it("renders the coarse-pointer drawer variant instead of the popover", () => {
    touchMocks.usesTouchDrawer = true;
    gitlabMocks.mrs = [makeMR()];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);
    expect(trigger.getAttribute("aria-haspopup")).toBe("dialog");
    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(trigger);
    expect(screen.getByTestId("mr-status-chip-drawer")).toBeTruthy();
    expect(screen.getByTestId(POPOVER_STUB_TESTID).textContent).toContain("!81");

    act(() => {
      fireEvent.click(screen.getByTestId("mr-status-chip-drawer-close"));
      vi.advanceTimersByTime(1000);
    });
    expect(screen.getByTestId("mr-status-chip-drawer").getAttribute("data-state")).toBe("closed");
  });
});
