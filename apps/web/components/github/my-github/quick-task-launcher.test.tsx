import { render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { Icon } from "@tabler/icons-react";
import type { GitHubIssue, GitHubPR } from "@/lib/types/github";
import { qk } from "@/lib/query/keys";
import {
  repositoryId,
  workspaceId,
  type Repository,
  type Task,
  type Workflow,
  type WorkflowStep,
} from "@/lib/types/http";
import { QuickTaskLauncher, type LaunchPayload, type TaskPreset } from "./quick-task-launcher";

const NOW = "2026-07-01T00:00:00Z";
const WORKSPACE_ID = "workspace-1";
const WORKFLOW_ID = "workflow-1";
const PR_HEAD_BRANCH = "feature/adding-a-download-ot-5sl";
const TASK_WORKTREE_ROOT = "/root/.kandev/tasks";
const TASK_WORKTREE_PATH = "/root/.kandev/tasks/pr-1541-fix-skip-cle_3bm/kdlbs-kandev";
const REPO_URL = "https://github.com/kdlbs/kandev/pull/1567";
const ISSUE_URL = "https://github.com/kdlbs/kandev/issues/1567";
const LOCAL_REPO_ID = "local-repo";
let queryClient = createQueryClient();

function createQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function renderWithQueryClient(node: ReactNode) {
  return render(node, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    ),
  });
}

const mocks = vi.hoisted(() => ({
  dialogProps: undefined as
    | { initialValues?: Record<string, unknown>; onSuccess?: (task: Task) => void }
    | undefined,
  push: vi.fn(),
  createTaskPR: vi.fn(),
  linkTaskIssue: vi.fn(),
}));

vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: (props: { initialValues?: Record<string, unknown> }) => {
    mocks.dialogProps = props;
    return null;
  },
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  createTaskPR: mocks.createTaskPR,
  linkTaskIssue: mocks.linkTaskIssue,
}));

const preset: TaskPreset = {
  id: "review",
  label: "Review",
  hint: "Review the PR",
  icon: (() => null) as unknown as Icon,
  prompt: ({ url }) => `Review ${url}`,
};

const workflow = {
  id: WORKFLOW_ID,
  workspace_id: WORKSPACE_ID,
  name: "Workflow",
  created_at: NOW,
  updated_at: NOW,
} as Workflow;

const step = {
  id: "step-1",
  workflow_id: WORKFLOW_ID,
  name: "Backlog",
  position: 1,
  color: "gray",
  created_at: NOW,
  updated_at: NOW,
} as WorkflowStep;

function pr(overrides: Partial<GitHubPR> = {}): GitHubPR {
  return {
    number: 1567,
    title: "feat: add Download option",
    url: REPO_URL,
    html_url: REPO_URL,
    state: "open",
    head_branch: PR_HEAD_BRANCH,
    base_branch: "main",
    author_login: "contributor",
    repo_owner: "kdlbs",
    repo_name: "kandev",
    draft: false,
    mergeable: true,
    additions: 0,
    deletions: 0,
    requested_reviewers: [],
    created_at: NOW,
    updated_at: NOW,
    merged_at: null,
    closed_at: null,
    ...overrides,
  };
}

function issue(overrides: Partial<GitHubIssue> = {}): GitHubIssue {
  return {
    number: 1567,
    title: "add Download option",
    body: "",
    url: ISSUE_URL,
    html_url: ISSUE_URL,
    state: "open",
    author_login: "contributor",
    repo_owner: "kdlbs",
    repo_name: "kandev",
    labels: [],
    assignees: [],
    created_at: NOW,
    updated_at: NOW,
    closed_at: null,
    ...overrides,
  };
}

type RepoOverrides = Omit<Partial<Repository>, "id" | "workspace_id"> & { id: string };

function repo(overrides: RepoOverrides): Repository {
  const { id, ...rest } = overrides;
  return {
    id: repositoryId(id),
    workspace_id: workspaceId(WORKSPACE_ID),
    name: "kdlbs/kandev",
    source_type: "local",
    local_path: "/work/kandev",
    provider: "github",
    provider_repo_id: "",
    provider_owner: "kdlbs",
    provider_name: "kandev",
    default_branch: "main",
    worktree_branch_prefix: "feature/",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: NOW,
    updated_at: NOW,
    ...rest,
  };
}

function renderLauncher(repositories: Repository[], prOverrides: Partial<GitHubPR> = {}) {
  const payload: LaunchPayload = { kind: "pr", pr: pr(prOverrides), preset };
  renderWithQueryClient(
    <QuickTaskLauncher
      workspaceId={WORKSPACE_ID}
      workflows={[workflow]}
      steps={[step]}
      repositories={repositories}
      payload={payload}
      onClose={vi.fn()}
    />,
  );
  return mocks.dialogProps?.initialValues;
}

function renderIssueLauncher(
  repositories: Repository[],
  issueOverrides: Partial<GitHubIssue> = {},
) {
  const payload: LaunchPayload = { kind: "issue", issue: issue(issueOverrides), preset };
  renderWithQueryClient(
    <QuickTaskLauncher
      workspaceId={WORKSPACE_ID}
      workflows={[workflow]}
      steps={[step]}
      repositories={repositories}
      payload={payload}
      onClose={vi.fn()}
    />,
  );
  return mocks.dialogProps?.initialValues;
}

afterEach(() => {
  queryClient.clear();
  queryClient = createQueryClient();
  mocks.dialogProps = undefined;
  mocks.push.mockClear();
  mocks.createTaskPR.mockClear();
  mocks.linkTaskIssue.mockClear();
});

describe("QuickTaskLauncher repository defaults", () => {
  it("falls back to Remote mode when the only matching repo is a task worktree path", () => {
    const initialValues = renderLauncher([
      repo({ id: "task-worktree", local_path: TASK_WORKTREE_PATH }),
    ]);

    expect(initialValues).toMatchObject({
      githubUrl: REPO_URL,
      branch: PR_HEAD_BRANCH,
      checkoutBranch: PR_HEAD_BRANCH,
      prNumber: 1567,
      prBaseBranch: "main",
    });
    expect(initialValues?.repositoryId).toBeUndefined();
  });

  it("opens PR launches in Remote mode even when a provider-backed repo exists", () => {
    const initialValues = renderLauncher([
      repo({ id: "task-worktree", local_path: TASK_WORKTREE_PATH }),
      repo({
        id: "provider-repo",
        source_type: "provider",
        local_path: "",
      }),
    ]);

    expect(initialValues).toMatchObject({
      githubUrl: REPO_URL,
      branch: PR_HEAD_BRANCH,
      checkoutBranch: PR_HEAD_BRANCH,
      prNumber: 1567,
      prBaseBranch: "main",
    });
    expect(initialValues?.repositoryId).toBeUndefined();
  });

  it("still preselects an ordinary matching local GitHub repo for issues", () => {
    const initialValues = renderIssueLauncher([
      repo({ id: LOCAL_REPO_ID, local_path: "/work/kandev" }),
    ]);

    expect(initialValues).toMatchObject({
      repositoryId: LOCAL_REPO_ID,
    });
    expect(initialValues?.githubUrl).toBeUndefined();
  });

  it("rejects task worktree roots without a trailing slash", () => {
    const initialValues = renderIssueLauncher([
      repo({ id: "task-worktree-root", local_path: TASK_WORKTREE_ROOT }),
    ]);

    expect(initialValues).toMatchObject({
      githubUrl: "github.com/kdlbs/kandev",
    });
    expect(initialValues?.repositoryId).toBeUndefined();
  });

  it("derives a PR URL when the GitHub payload omits URL fields", () => {
    const initialValues = renderLauncher(
      [repo({ id: "task-worktree", local_path: TASK_WORKTREE_PATH })],
      { html_url: "", url: "" },
    );

    expect(initialValues).toMatchObject({
      githubUrl: REPO_URL,
      branch: PR_HEAD_BRANCH,
      checkoutBranch: PR_HEAD_BRANCH,
      prNumber: 1567,
      prBaseBranch: "main",
    });
    expect(initialValues?.repositoryId).toBeUndefined();
  });
});

describe("QuickTaskLauncher issue linking", () => {
  it("links and immediately caches a newly created task for the launched issue", async () => {
    const link = {
      task_id: "task-1",
      task_title: "Review: add Download option",
      owner: "kdlbs",
      repo: "kandev",
      issue_number: 1567,
      issue_url: ISSUE_URL,
      issue_title: "add Download option",
    };
    mocks.linkTaskIssue.mockResolvedValue(link);
    renderIssueLauncher([repo({ id: LOCAL_REPO_ID })]);

    mocks.dialogProps?.onSuccess?.({ id: "task-1" } as Task);

    expect(mocks.linkTaskIssue).toHaveBeenCalledWith("task-1", { issue: ISSUE_URL });
    await waitFor(() => {
      expect(queryClient.getQueryData(qk.integrations.github.issues(WORKSPACE_ID))).toEqual({
        "task-1": link,
      });
    });
    expect(mocks.push).toHaveBeenCalledWith("/tasks/task-1");
  });

  it("navigates when issue linking fails", async () => {
    mocks.linkTaskIssue.mockRejectedValueOnce(new Error("offline"));
    renderIssueLauncher([repo({ id: LOCAL_REPO_ID })]);

    mocks.dialogProps?.onSuccess?.({ id: "task-1" } as Task);

    expect(mocks.push).toHaveBeenCalledWith("/tasks/task-1");
    await waitFor(() => expect(mocks.linkTaskIssue).toHaveBeenCalledTimes(1));
    expect(queryClient.getQueryData(qk.integrations.github.issues(WORKSPACE_ID))).toBeUndefined();
  });
});
