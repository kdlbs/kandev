import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type {
  GitLabMRFeedback,
  GitLabPipelineJob,
  TaskMR,
  TaskMRAutomationOptions,
} from "@/lib/types/gitlab";
import { MRCIPopover } from "./mr-ci-popover";

const workspace = "ci-workspace";
const task = "ci-task";
const host = "https://gitlab.example";
const project = "team/service";
const mrUrl = `${host}/${project}/-/merge_requests/23`;
const optionalUrl = `${host}/jobs/optional`;
const dominantUrl = `${host}/jobs/required`;
const timestamp = "2026-10-13T08:00:00Z";
const stageGroupsTestId = "mr-pipeline-stage-groups";
const acceptedPair = "2/2 (100%)";

function job(overrides: Partial<GitLabPipelineJob> = {}): GitLabPipelineJob {
  return {
    id: 2,
    name: "Optional deployment",
    stage: "deploy",
    status: "manual",
    allow_failure: true,
    web_url: optionalUrl,
    ...overrides,
  };
}

function association(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "ci-association",
    task_id: task,
    repository_id: "ci-repository",
    host,
    project_path: project,
    mr_iid: 23,
    mr_url: mrUrl,
    mr_title: "CI classification",
    head_branch: "feature",
    base_branch: "main",
    author_username: "author",
    state: "open",
    approval_state: "approved",
    pipeline_state: "success",
    merge_status: "can_be_merged",
    detailed_merge_status: "mergeable",
    draft: false,
    approval_count: 1,
    required_approvals: 1,
    pipeline_jobs_total: 2,
    pipeline_jobs_pass: 2,
    reviewer_count: 1,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: timestamp,
    updated_at: timestamp,
    ...overrides,
  };
}

function feedback(jobs: GitLabPipelineJob[] | undefined, status = "success"): GitLabMRFeedback {
  return {
    mr: {
      id: 123,
      iid: 23,
      project_id: 10,
      title: "CI classification",
      url: mrUrl,
      web_url: mrUrl,
      state: "opened",
      head_branch: "feature",
      head_sha: "abc123",
      base_branch: "main",
      author_username: "author",
      project_namespace: "team",
      project_path: project,
      body: "Check optional deployment",
      draft: false,
      merge_status: "can_be_merged",
      has_conflicts: false,
      additions: 0,
      deletions: 0,
      reviewers: [],
      assignees: [],
      created_at: timestamp,
      updated_at: timestamp,
    },
    approvals: [{ username: "reviewer", created_at: timestamp }],
    discussions: [],
    pipelines: [
      {
        id: 52,
        iid: 52,
        status,
        source: "push",
        ref: "feature",
        sha: "abc123",
        web_url: `${host}/pipelines/52`,
        jobs_total: jobs?.length ?? 2,
        jobs_passing: 2,
        jobs,
      },
    ],
    has_issues: status === "failed",
  };
}

function automation(): TaskMRAutomationOptions {
  const switches = {
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
  };
  return {
    task_id: task,
    ...switches,
    automation_revision: 1,
    auto_fix_prompt_override: null,
    auto_fix_max_rounds: 10,
    effective_auto_fix_prompt: "Review CI feedback",
    using_default_prompt: true,
    review_reviewer_username: "reviewer",
    updated_at: timestamp,
    mr_states: [],
    mr_options: [
      {
        task_id: task,
        repository_id: "ci-repository",
        project_path: project,
        mr_iid: 23,
        ...switches,
        created_at: timestamp,
        updated_at: timestamp,
      },
    ],
  };
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

let resolveFeedback: ((response: Response) => void) | undefined;
let fetched: URL[];

beforeEach(() => {
  fetched = [];
  const held = new Promise<Response>((resolve) => {
    resolveFeedback = resolve;
  });
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), "http://localhost");
    fetched.push(url);
    expect(init?.method ?? "GET").toBe("GET");
    if (url.pathname === "/api/v1/gitlab/mrs/feedback") return held;
    if (url.pathname === "/api/v1/gitlab/mrs/files") return Promise.resolve(json({ files: [] }));
    if (url.pathname === "/api/v1/gitlab/mrs/commits")
      return Promise.resolve(json({ commits: [] }));
    if (url.pathname === `/api/v1/gitlab/tasks/${task}/mr-automation`)
      return Promise.resolve(json(automation()));
    throw new Error(`Unexpected read: ${url.pathname}`);
  });
});

afterEach(async () => {
  await act(async () => {
    resolveFeedback?.(json(feedback(undefined)));
  });
  cleanup();
  vi.unstubAllGlobals();
});

function mount(mr = association()) {
  return render(
    <StateProvider initialState={{ workspaces: { activeId: workspace, items: [] } }}>
      <TooltipProvider>
        <ToastProvider>
          <MRCIPopover
            mr={mr}
            taskId={task}
            enabled
            canLink={false}
            onLink={() => {}}
            onUnlink={() => {}}
          />
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
}

async function deliver(body: GitLabMRFeedback, status = 200) {
  await act(async () => {
    resolveFeedback?.(json(status === 200 ? body : { error: "Unavailable" }, status));
  });
}

function expectPassRate(text: string, width: string) {
  const bar = screen.getByTestId("mr-pipeline-progress");
  expect(bar.textContent).toContain(text);
  expect((bar.querySelector('[data-segment="passed"]') as HTMLElement).style.width).toBe(width);
}

function expectStage(counts: string, color: string, url: string) {
  const groups = screen.getByTestId(stageGroupsTestId);
  const row = within(groups).getByRole("link", { name: `deploy ${counts}` });
  expect(row.getAttribute("href")).toBe(url);
  expect(row.querySelector(`svg.${color}`)).not.toBeNull();
}

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.16
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.19
describe("MRCIPopover optional manual CI", () => {
  it("uses real live jobs for 2/2 and accepted deployment instead of stale cached counts", async () => {
    mount(association({ pipeline_jobs_pass: 0 }));
    const jobs = [job({ id: 1, stage: "test", status: "success", allow_failure: false }), job()];
    await deliver(feedback(jobs));
    await waitFor(() => expectPassRate(acceptedPair, "100%"));
    expectStage("1/1", "text-emerald-500", optionalUrl);
    const read = fetched.find((url) => url.pathname.endsWith("/feedback"));
    expect(read?.searchParams.get("workspace_id")).toBe(workspace);
    expect(read?.searchParams.get("project")).toBe(project);
    expect(read?.searchParams.get("iid")).toBe("23");
    expect(read?.searchParams.get("expected_host")).toBe(host);
    expect(jobs[1]).toMatchObject({ status: "manual", allow_failure: true });
  });

  it("retains cached accepted counts before feedback and when no live job list exists", async () => {
    mount();
    expectPassRate(acceptedPair, "100%");
    expect(screen.queryByTestId(stageGroupsTestId)).toBeNull();
    await deliver(feedback(undefined));
    expectPassRate(acceptedPair, "100%");
    expect(screen.queryByTestId(stageGroupsTestId)).toBeNull();
  });

  it("retains the aggregate fallback when the initial feedback read is unavailable", async () => {
    mount();
    await deliver(feedback(undefined), 503);
    expectPassRate(acceptedPair, "100%");
    expect(screen.queryByTestId(stageGroupsTestId)).toBeNull();
  });

  // @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.17
  // @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.18
  it.each([
    ["manual", false, "text-yellow-500"],
    ["running", true, "text-yellow-500"],
    ["pending", true, "text-yellow-500"],
    ["failed", false, "text-red-500"],
  ] as const)(
    "keeps %s allowance %s dominant beside optional manual",
    async (status, allow_failure, color) => {
      mount();
      await deliver(feedback([job(), job({ id: 3, status, allow_failure, web_url: dominantUrl })]));
      await waitFor(() => expectPassRate("1/2 (50%)", "50%"));
      expectStage("1/2", color, dominantUrl);
    },
  );

  it("does not enable merge from accepted counts when the pipeline is not successful", async () => {
    const view = mount(association({ pipeline_state: "pending" }));
    await deliver(feedback([job(), job({ id: 1, status: "skipped" })], "running"));
    await waitFor(() => expectPassRate(acceptedPair, "100%"));
    expect(screen.queryByTestId("mr-merge-button")).toBeNull();
    view.unmount();
    mount(association({ pipeline_state: "success" }));
    expect(screen.getByTestId("mr-merge-button")).toBeTruthy();
  });
});
