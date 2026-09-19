import { createElement } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useHoverPopover } from "@/hooks/domains/github/use-hover-popover";
import { MRStatusChip } from "./mr-status-chip";
import type {
  TaskMR,
  TaskMRAutomationOptions,
  TaskMRAutomationOptionsForMR,
} from "@/lib/types/gitlab";

const OPEN_DELAY_MS = 150;
const CHIP_TESTID = "mr-status-chip";
const DATA_MR_IID = "data-mr-iid";
const DATA_SELECTION_FROZEN = "data-selection-frozen";
const DATA_MR_COUNT = "data-mr-count";
const DATA_STATUS = "data-status";
const POPOVER_STUB_TESTID = "mr-ci-popover-stub";

const gitlabMocks = vi.hoisted(() => ({ mrs: [] as TaskMR[] }));
const touchMocks = vi.hoisted(() => ({ usesTouchDrawer: false }));
const automationMocks = vi.hoisted(() => ({
  options: null as TaskMRAutomationOptions | null,
}));
const dialogMocks = vi.hoisted(() => ({ lastProps: null as { open: boolean } | null }));
const unlinkMock = vi.hoisted(() => vi.fn(async () => {}));
const popoverRenderMRIIDs = vi.hoisted(() => [] as number[]);

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
  useWorkspaceMRs: () => undefined,
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
  }) => {
    popoverRenderMRIIDs.push(mr.mr_iid);
    return (
      <div data-testid={POPOVER_STUB_TESTID}>
        popover for !{mr.mr_iid}
        <button type="button" onClick={onLink}>
          link another
        </button>
        <button type="button" onClick={onUnlink}>
          unlink
        </button>
      </div>
    );
  },
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
    mr_options: [],
    mr_states: [],
    ...overrides,
  };
}

/**
 * One `mr_options` row for the default `makeMR()` MR. The badges read each
 * MR's own switches (the top-level booleans are an aggregate over every
 * linked MR), so a fixture with open MRs must carry a matching row —
 * `mr_options: []` alongside an open MR is a state the backend cannot
 * produce, since it emits one entry per linked MR.
 */
function makeMROptions(
  overrides: Partial<TaskMRAutomationOptionsForMR> = {},
): TaskMRAutomationOptionsForMR {
  return {
    task_id: "task-1",
    repository_id: "",
    project_path: "group/project",
    mr_iid: 81,
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function resetChipMocks() {
  vi.useFakeTimers();
  touchMocks.usesTouchDrawer = false;
  automationMocks.options = null;
  dialogMocks.lastProps = null;
  unlinkMock.mockClear();
  popoverRenderMRIIDs.length = 0;
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
    expect(trigger.getAttribute(DATA_STATUS)).toBe("failed");
    expect(trigger.getAttribute(DATA_MR_COUNT)).toBe("1");
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("81");
    expect(trigger.getAttribute("data-mr-state")).toBe("open");
    expect(trigger.getAttribute("data-mr-ready-to-merge")).toBe("false");
    expect(trigger.getAttribute(DATA_SELECTION_FROZEN)).toBe("false");
  });

  it("selects the highest-ranked open MR and reports the total open count", () => {
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }),
      makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }),
      makeMR({ id: "c", mr_iid: 3, state: "merged" }),
    ];
    render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);
    expect(trigger.getAttribute(DATA_STATUS)).toBe("failed");
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("2");
    // Terminal MR is not counted toward data-mr-count.
    expect(trigger.getAttribute(DATA_MR_COUNT)).toBe("2");
  });

  it("renders the auto-fix badge at round 0 with no lifecycle state, and the auto-merge badge", () => {
    gitlabMocks.mrs = [makeMR()];
    automationMocks.options = makeAutomation({
      auto_fix_enabled: true,
      auto_fix_max_rounds: 5,
      auto_merge_enabled: true,
      mr_options: [makeMROptions({ auto_fix_enabled: true, auto_merge_enabled: true })],
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
    const { rerender } = render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);

    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });

    expect(screen.getByTestId(POPOVER_STUB_TESTID).textContent).toContain("!2");
    expect(trigger.getAttribute(DATA_SELECTION_FROZEN)).toBe("true");
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("2");

    // Store update makes MR "a" the new live selection (now also failed,
    // and it outranks "b" by the mr_iid tiebreak) while the popover stays
    // open on "b": data-status tracks the live selection, data-mr-iid and
    // data-selection-frozen stay frozen on "b" (spec: Selection - freezing
    // while the disclosure is open). A `rerender()` is required — mutating
    // the hoisted mock array alone does not re-render the component.
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "failure" }),
      makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }),
    ];
    act(() => {
      rerender(createElement(MRStatusChip, { taskId: "task-1" }));
    });

    expect(trigger.getAttribute(DATA_MR_IID)).toBe("2");
    expect(trigger.getAttribute(DATA_SELECTION_FROZEN)).toBe("true");
    expect(trigger.getAttribute(DATA_STATUS)).toBe("failed");
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
    expect(trigger.getAttribute(DATA_STATUS)).toBe("running");
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

describe("MRStatusChip freeze and forced close", () => {
  beforeEach(resetChipMocks);
  afterEach(teardownChipMocks);

  // spec: Failure modes / Idempotency and re-render — "if the transitioned MR
  // is the frozen one, the popover or drawer closes; if another open MR
  // remains the trigger re-selects it." Exercises useFrozenChipSelection's
  // shouldForceClose path (mr-status-chip-selection.ts:98), which previously
  // had zero coverage (Review round 1 finding #4).
  it("force-closes the popover and re-selects the remaining open MR when the frozen MR goes terminal", () => {
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }), // running, rank 4
      makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }), // failed, rank 5 — gets frozen
    ];
    const { rerender } = render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);

    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });

    expect(screen.getByTestId(POPOVER_STUB_TESTID)).toBeTruthy();
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("2");
    expect(trigger.getAttribute(DATA_SELECTION_FROZEN)).toBe("true");

    // The frozen MR ("b") merges elsewhere while the popover is open; "a"
    // remains the task's only open MR.
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }),
      makeMR({ id: "b", mr_iid: 2, state: "merged" }),
    ];
    act(() => {
      rerender(createElement(MRStatusChip, { taskId: "task-1" }));
    });

    expect(screen.queryByTestId(POPOVER_STUB_TESTID)).toBeNull();
    expect(trigger.getAttribute(DATA_MR_COUNT)).toBe("1");
    expect(trigger.getAttribute(DATA_MR_IID)).toBe("1");
    expect(trigger.getAttribute(DATA_SELECTION_FROZEN)).toBe("false");
  });

  // Review round 1 finding #6 (codex): shouldForceClose and actedOnMR's
  // fallback to the live selection are computed synchronously in the same
  // render as the store update, but closing the disclosure is a `useEffect`
  // that only fires after that render commits. Without the `!shouldForceClose`
  // guard in MRStatusChipPopover/Drawer, MRCIPopover renders — with active
  // merge/unlink controls — for the swapped-in MR ("a") for one commit before
  // the effect closes the popover. Tracking every mr_iid MRCIPopover was ever
  // rendered with (not just the final settled DOM, which act() would show as
  // already-closed either way) is what actually catches this.
  it("never renders MRCIPopover for the swapped-in MR during the forced-close transition", () => {
    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }),
      makeMR({ id: "b", mr_iid: 2, pipeline_state: "failure" }), // frozen
    ];
    const { rerender } = render(createElement(MRStatusChip, { taskId: "task-1" }));
    const trigger = screen.getByTestId(CHIP_TESTID);

    act(() => {
      fireEvent.mouseEnter(trigger);
      vi.advanceTimersByTime(OPEN_DELAY_MS);
    });
    expect(popoverRenderMRIIDs).toEqual([2]);

    gitlabMocks.mrs = [
      makeMR({ id: "a", mr_iid: 1, pipeline_state: "pending" }),
      makeMR({ id: "b", mr_iid: 2, state: "merged" }),
    ];
    act(() => {
      rerender(createElement(MRStatusChip, { taskId: "task-1" }));
    });

    // Across the whole transition (freeze open -> b vanishes -> popover
    // closes), MRCIPopover is only ever rendered for "b" (the MR the user
    // opened the popover on) — never for "a" (the swapped-in live MR).
    expect(popoverRenderMRIIDs).not.toContain(1);
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
