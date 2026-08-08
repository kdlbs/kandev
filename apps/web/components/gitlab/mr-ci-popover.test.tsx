import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type {
  GitLabMRDiscussion,
  GitLabMRFeedback,
  GitLabPipelineJob,
  TaskMR,
} from "@/lib/types/gitlab";

const feedbackMocks = vi.hoisted(() => ({
  feedback: null as GitLabMRFeedback | null,
  loading: false,
  revision: 0,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { activeId: string } }) => unknown) =>
    selector({ workspaces: { activeId: "ws-1" } }),
}));

vi.mock("@/hooks/domains/gitlab/use-mr-feedback", () => ({
  useMRFeedback: () => ({
    feedback: feedbackMocks.feedback,
    loading: feedbackMocks.loading,
    revision: feedbackMocks.revision,
    error: null,
    files: [],
    commits: [],
    refresh: vi.fn(),
  }),
}));

// Automation controls and the merge button have their own dedicated test
// files; this suite only verifies the popover composes them, via stubs.
vi.mock("./mr-automation-controls", () => ({
  MRAutomationControls: () => <div data-testid="mr-automation-controls-stub" />,
}));
vi.mock("./mr-merge-button", () => ({
  MRMergeButton: () => <div data-testid="mr-merge-button-stub" />,
}));

import { MRCIPopover } from "./mr-ci-popover";

function renderPopover(
  mr: TaskMR,
  overrides: Partial<{
    taskId: string;
    canLink: boolean;
    onLink: () => void;
    onUnlink: () => void;
  }> = {},
) {
  return render(
    <MRCIPopover
      mr={mr}
      taskId={overrides.taskId ?? "task-1"}
      enabled
      canLink={overrides.canLink ?? true}
      onLink={overrides.onLink ?? vi.fn()}
      onUnlink={overrides.onUnlink ?? vi.fn()}
    />,
  );
}

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "id",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "group/project",
    mr_iid: 7,
    mr_url: "",
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

function makeJob(overrides: Partial<GitLabPipelineJob> = {}): GitLabPipelineJob {
  return {
    id: 1,
    name: "unit",
    stage: "test",
    status: "success",
    allow_failure: false,
    ...overrides,
  };
}

function makeDiscussion(overrides: Partial<GitLabMRDiscussion> = {}): GitLabMRDiscussion {
  return {
    id: "d1",
    resolvable: true,
    resolved: false,
    notes: [],
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function makeFeedback(overrides: Partial<GitLabMRFeedback> = {}): GitLabMRFeedback {
  return {
    mr: { iid: 7 } as GitLabMRFeedback["mr"],
    approvals: [],
    discussions: [],
    pipelines: [],
    has_issues: false,
    ...overrides,
  };
}

const APPROVAL_ROW_TEST_ID = "mr-approval-row";

beforeEach(() => {
  feedbackMocks.feedback = null;
  feedbackMocks.loading = false;
  feedbackMocks.revision = 0;
});

afterEach(() => cleanup());

describe("MRCIPopover — pipeline (AC18/AC19)", () => {
  it("AC18: renders the pass-rate bar at the live job breakdown once feedback loads", () => {
    feedbackMocks.feedback = makeFeedback({
      pipelines: [
        {
          id: 1,
          iid: 1,
          status: "failed",
          source: "push",
          ref: "feature",
          sha: "abc",
          web_url: "",
          jobs_total: 10,
          jobs_passing: 6,
          jobs: [
            ...Array.from({ length: 6 }, (_, i) => makeJob({ id: i + 1, status: "success" })),
            ...Array.from({ length: 4 }, (_, i) => makeJob({ id: i + 7, status: "failed" })),
          ],
        },
      ],
    });
    renderPopover(makeMR({ pipeline_jobs_total: 10, pipeline_jobs_pass: 6 }));
    const bar = screen.getByTestId("mr-pipeline-progress");
    expect(bar.textContent).toContain("6/10");
    expect(bar.textContent).toContain("60%");
    const segment = bar.querySelector('[data-segment="passed"]') as HTMLElement;
    expect(segment.style.width).toBe("60%");
  });

  it("AC18: falls back to TaskMR's aggregate counts before feedback loads", () => {
    renderPopover(makeMR({ pipeline_jobs_total: 10, pipeline_jobs_pass: 6 }));
    const bar = screen.getByTestId("mr-pipeline-progress");
    expect(bar.textContent).toContain("6/10");
  });

  it("AC19: renders the empty state instead of the progress bar when there are zero jobs", () => {
    renderPopover(makeMR({ pipeline_jobs_total: 0, pipeline_jobs_pass: 0 }));
    expect(screen.queryByTestId("mr-pipeline-progress")).toBeNull();
    expect(screen.getByTestId("mr-pipeline-empty")).toBeTruthy();
  });
});

describe("MRCIPopover — approvals (AC20/AC21)", () => {
  it("AC20: renders Approved with a check icon when approval_count meets required_approvals", () => {
    renderPopover(makeMR({ approval_count: 2, required_approvals: 2 }));
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("Approved");
    expect(row.textContent).toContain("2 / 2");
  });

  it("AC20: renders Awaiting review when required_approvals is unmet", () => {
    renderPopover(makeMR({ approval_count: 0, required_approvals: 2 }));
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("Awaiting review");
    expect(row.textContent).toContain("0 / 2");
  });

  it("AC20: shows just the approval count with no required suffix when required_approvals is 0", () => {
    renderPopover(makeMR({ approval_count: 1, required_approvals: 0 }));
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("1");
    expect(row.textContent).not.toContain("/");
  });

  it("AC20: renders Approved (not Awaiting review) when required_approvals is 0 but the MR has an approval", () => {
    renderPopover(makeMR({ approval_count: 1, required_approvals: 0 }));
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("Approved");
    expect(row.textContent).not.toContain("Awaiting review");
  });

  it("AC20: renders Awaiting review when required_approvals is 0 and there are zero approvals", () => {
    renderPopover(makeMR({ approval_count: 0, required_approvals: 0 }));
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("Awaiting review");
  });

  it("AC21: appends an awaiting-count suffix when reviewers haven't approved yet", () => {
    renderPopover(makeMR({ approval_count: 1, required_approvals: 2, unapproved_reviewers: 1 }));
    expect(screen.getByTestId(APPROVAL_ROW_TEST_ID).textContent).toContain("1 awaiting");
  });

  it("AC21: omits the suffix when there are no unapproved reviewers", () => {
    renderPopover(makeMR({ approval_count: 2, required_approvals: 2, unapproved_reviewers: 0 }));
    expect(screen.getByTestId(APPROVAL_ROW_TEST_ID).textContent).not.toContain("awaiting");
  });
});

describe("MRCIPopover — discussions (AC22)", () => {
  it("AC22: renders the unresolved-comments row via count-based pluralization, singular", () => {
    feedbackMocks.feedback = makeFeedback({ discussions: [makeDiscussion()] });
    renderPopover(makeMR());
    expect(screen.getByTestId("mr-discussions-row").textContent).toContain("1 unresolved comment");
  });

  it("AC22: renders the unresolved-comments row via count-based pluralization, plural", () => {
    feedbackMocks.feedback = makeFeedback({
      discussions: [makeDiscussion({ id: "d1" }), makeDiscussion({ id: "d2" })],
    });
    renderPopover(makeMR());
    expect(screen.getByTestId("mr-discussions-row").textContent).toContain("2 unresolved comments");
  });

  it("AC22: omits the discussions row when there are zero unresolved discussions", () => {
    feedbackMocks.feedback = makeFeedback({
      discussions: [makeDiscussion({ resolved: true })],
    });
    renderPopover(makeMR());
    expect(screen.queryByTestId("mr-discussions-row")).toBeNull();
  });
});

describe("MRCIPopover — header actions, automation, and link-another", () => {
  it("opens the MR externally via the header's Open in GitLab link", () => {
    renderPopover(makeMR({ mr_url: "https://gitlab.example/group/project/-/merge_requests/7" }));
    const link = screen.getByTestId("mr-popover-open-in-gitlab");
    expect(link.getAttribute("href")).toBe(
      "https://gitlab.example/group/project/-/merge_requests/7",
    );
    expect(link.getAttribute("target")).toBe("_blank");
  });

  it("calls onUnlink when the header's unlink button is clicked", () => {
    const onUnlink = vi.fn();
    renderPopover(makeMR(), { onUnlink });
    fireEvent.click(screen.getByRole("button", { name: /unlink/i }));
    expect(onUnlink).toHaveBeenCalledTimes(1);
  });

  it("calls onOpenDetailPanel when the title is clicked", () => {
    const onOpenDetailPanel = vi.fn();
    render(
      <MRCIPopover
        mr={makeMR()}
        taskId="task-1"
        enabled
        canLink
        onLink={vi.fn()}
        onUnlink={vi.fn()}
        onOpenDetailPanel={onOpenDetailPanel}
      />,
    );
    fireEvent.click(screen.getByTestId("mr-popover-title"));
    expect(onOpenDetailPanel).toHaveBeenCalledTimes(1);
  });

  it("renders the Automation section and a merge action alongside the CI summary", () => {
    renderPopover(makeMR());
    expect(screen.getByTestId("mr-automation-controls-stub")).toBeTruthy();
    expect(screen.getByTestId("mr-merge-button-stub")).toBeTruthy();
  });

  it("shows Link another merge request when canLink is true, and calls onLink", () => {
    const onLink = vi.fn();
    renderPopover(makeMR(), { canLink: true, onLink });
    fireEvent.click(screen.getByTestId("mr-popover-link-another"));
    expect(onLink).toHaveBeenCalledTimes(1);
  });

  it("hides Link another merge request when canLink is false", () => {
    renderPopover(makeMR(), { canLink: false });
    expect(screen.queryByTestId("mr-popover-link-another")).toBeNull();
  });
});
