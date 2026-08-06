import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
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

import { MRCIPopover } from "./mr-ci-popover";

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
    render(<MRCIPopover mr={makeMR({ pipeline_jobs_total: 10, pipeline_jobs_pass: 6 })} enabled />);
    const bar = screen.getByTestId("mr-pipeline-progress");
    expect(bar.textContent).toContain("6/10");
    expect(bar.textContent).toContain("60%");
    const segment = bar.querySelector('[data-segment="passed"]') as HTMLElement;
    expect(segment.style.width).toBe("60%");
  });

  it("AC18: falls back to TaskMR's aggregate counts before feedback loads", () => {
    render(<MRCIPopover mr={makeMR({ pipeline_jobs_total: 10, pipeline_jobs_pass: 6 })} enabled />);
    const bar = screen.getByTestId("mr-pipeline-progress");
    expect(bar.textContent).toContain("6/10");
  });

  it("AC19: renders the empty state instead of the progress bar when there are zero jobs", () => {
    render(<MRCIPopover mr={makeMR({ pipeline_jobs_total: 0, pipeline_jobs_pass: 0 })} enabled />);
    expect(screen.queryByTestId("mr-pipeline-progress")).toBeNull();
    expect(screen.getByTestId("mr-pipeline-empty")).toBeTruthy();
  });
});

describe("MRCIPopover — approvals (AC20/AC21)", () => {
  it("AC20: renders Approved with a check icon when approval_count meets required_approvals", () => {
    render(<MRCIPopover mr={makeMR({ approval_count: 2, required_approvals: 2 })} enabled />);
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("Approved");
    expect(row.textContent).toContain("2 / 2");
  });

  it("AC20: renders Awaiting review when required_approvals is unmet", () => {
    render(<MRCIPopover mr={makeMR({ approval_count: 0, required_approvals: 2 })} enabled />);
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("Awaiting review");
    expect(row.textContent).toContain("0 / 2");
  });

  it("AC20: shows just the approval count with no required suffix when required_approvals is 0", () => {
    render(<MRCIPopover mr={makeMR({ approval_count: 1, required_approvals: 0 })} enabled />);
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("1");
    expect(row.textContent).not.toContain("/");
  });

  it("AC20: renders Approved (not Awaiting review) when required_approvals is 0 but the MR has an approval", () => {
    render(<MRCIPopover mr={makeMR({ approval_count: 1, required_approvals: 0 })} enabled />);
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("Approved");
    expect(row.textContent).not.toContain("Awaiting review");
  });

  it("AC20: renders Awaiting review when required_approvals is 0 and there are zero approvals", () => {
    render(<MRCIPopover mr={makeMR({ approval_count: 0, required_approvals: 0 })} enabled />);
    const row = screen.getByTestId(APPROVAL_ROW_TEST_ID);
    expect(row.textContent).toContain("Awaiting review");
  });

  it("AC21: appends an awaiting-count suffix when reviewers haven't approved yet", () => {
    render(
      <MRCIPopover
        mr={makeMR({ approval_count: 1, required_approvals: 2, unapproved_reviewers: 1 })}
        enabled
      />,
    );
    expect(screen.getByTestId(APPROVAL_ROW_TEST_ID).textContent).toContain("1 awaiting");
  });

  it("AC21: omits the suffix when there are no unapproved reviewers", () => {
    render(
      <MRCIPopover
        mr={makeMR({ approval_count: 2, required_approvals: 2, unapproved_reviewers: 0 })}
        enabled
      />,
    );
    expect(screen.getByTestId(APPROVAL_ROW_TEST_ID).textContent).not.toContain("awaiting");
  });
});

describe("MRCIPopover — discussions (AC22)", () => {
  it("AC22: renders the unresolved-comments row via count-based pluralization, singular", () => {
    feedbackMocks.feedback = makeFeedback({ discussions: [makeDiscussion()] });
    render(<MRCIPopover mr={makeMR()} enabled />);
    expect(screen.getByTestId("mr-discussions-row").textContent).toContain("1 unresolved comment");
  });

  it("AC22: renders the unresolved-comments row via count-based pluralization, plural", () => {
    feedbackMocks.feedback = makeFeedback({
      discussions: [makeDiscussion({ id: "d1" }), makeDiscussion({ id: "d2" })],
    });
    render(<MRCIPopover mr={makeMR()} enabled />);
    expect(screen.getByTestId("mr-discussions-row").textContent).toContain("2 unresolved comments");
  });

  it("AC22: omits the discussions row when there are zero unresolved discussions", () => {
    feedbackMocks.feedback = makeFeedback({
      discussions: [makeDiscussion({ resolved: true })],
    });
    render(<MRCIPopover mr={makeMR()} enabled />);
    expect(screen.queryByTestId("mr-discussions-row")).toBeNull();
  });
});
