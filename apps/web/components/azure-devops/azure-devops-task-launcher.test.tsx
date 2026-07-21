import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Repository, Workflow, WorkflowStep } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  associate: vi.fn(),
  cache: vi.fn(),
  close: vi.fn(),
  push: vi.fn(),
  setTaskPullRequest: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("sonner", () => ({ toast: { error: mocks.toastError } }));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  associateAzureDevOpsPullRequest: mocks.associate,
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-task-pull-requests", () => ({
  cacheAzureDevOpsTaskPullRequest: mocks.cache,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      setAzureDevOpsTaskPullRequest: typeof mocks.setTaskPullRequest;
    }) => unknown,
  ) => selector({ setAzureDevOpsTaskPullRequest: mocks.setTaskPullRequest }),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
}));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: ({ onSuccess }: { onSuccess?: (task: { id: string }) => void }) => (
    <button type="button" onClick={() => onSuccess?.({ id: "task-1" })}>
      Create task
    </button>
  ),
}));

import { AzureDevOpsTaskLauncher } from "./azure-devops-task-launcher";

const timestamp = "2026-07-18T00:00:00Z";
const workflow: Workflow = {
  id: "workflow-1" as Workflow["id"],
  workspace_id: "workspace-1" as Workflow["workspace_id"],
  name: "Review",
  created_at: timestamp,
  updated_at: timestamp,
};
const step: WorkflowStep = {
  id: "step-1",
  workflow_id: workflow.id,
  name: "Review",
  position: 0,
  color: "blue",
  created_at: timestamp,
  updated_at: timestamp,
};
const repository: Repository = {
  id: "repo-1" as Repository["id"],
  workspace_id: "workspace-1" as Repository["workspace_id"],
  name: "app",
  source_type: "git",
  local_path: "/repo",
  provider: "azure_devops",
  provider_repo_id: "azure-repo-1",
  provider_owner: "project-1",
  provider_name: "Azure DevOps",
  default_branch: "main",
  worktree_branch_prefix: "",
  pull_before_worktree: false,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: timestamp,
  updated_at: timestamp,
};
const pullRequest = {
  id: 42,
  title: "Review Azure integration",
  status: "active",
  isDraft: false,
  sourceBranch: "refs/heads/feature/azure",
  targetBranch: "refs/heads/main",
  author: { id: "author-1", displayName: "Ada" },
  projectId: "project-1",
  projectName: "Platform",
  repositoryId: "azure-repo-1",
  repositoryName: "app",
  webUrl: "https://dev.azure.com/acme/project/_git/app/pullrequest/42",
  apiUrl: "https://dev.azure.com/acme/project/_apis/git/pullrequests/42",
};

function renderLauncher() {
  return render(
    <AzureDevOpsTaskLauncher
      workspaceId="workspace-1"
      workflows={[workflow]}
      steps={[step]}
      repositories={[repository]}
      payload={{ kind: "pull-request", pullRequest }}
      onClose={mocks.close}
    />,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(cleanup);

describe("AzureDevOpsTaskLauncher", () => {
  it("updates the cache and store after associating the created task", async () => {
    const linked = { id: "link-1" };
    mocks.associate.mockResolvedValue(linked);
    renderLauncher();

    fireEvent.click(screen.getByRole("button", { name: "Create task" }));

    await waitFor(() => expect(mocks.cache).toHaveBeenCalledWith("workspace-1", "task-1", linked));
    expect(mocks.setTaskPullRequest).toHaveBeenCalledWith("task-1", linked);
    expect(mocks.push).toHaveBeenCalledWith("/tasks/task-1");
  });

  it("reports a pull request association failure", async () => {
    mocks.associate.mockRejectedValue(new Error("Azure association failed"));
    renderLauncher();

    fireEvent.click(screen.getByRole("button", { name: "Create task" }));

    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("Azure association failed"));
    expect(mocks.close).toHaveBeenCalled();
    expect(mocks.push).toHaveBeenCalledWith("/tasks/task-1");
  });
});
