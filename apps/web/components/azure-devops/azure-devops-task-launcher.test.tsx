import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Repository, Workflow, WorkflowStep } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  associate: vi.fn(),
  associateWorkItem: vi.fn(),
  cache: vi.fn(),
  cacheWorkItem: vi.fn(),
  close: vi.fn(),
  push: vi.fn(),
  setTaskPullRequest: vi.fn(),
  setTaskWorkItem: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("sonner", () => ({ toast: { error: mocks.toastError } }));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  associateAzureDevOpsPullRequest: mocks.associate,
  associateAzureDevOpsWorkItem: mocks.associateWorkItem,
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-task-pull-requests", () => ({
  cacheAzureDevOpsTaskPullRequest: mocks.cache,
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-task-work-items", () => ({
  cacheAzureDevOpsTaskWorkItem: mocks.cacheWorkItem,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      setAzureDevOpsTaskPullRequest: typeof mocks.setTaskPullRequest;
      setAzureDevOpsTaskWorkItem: typeof mocks.setTaskWorkItem;
    }) => unknown,
  ) =>
    selector({
      setAzureDevOpsTaskPullRequest: mocks.setTaskPullRequest,
      setAzureDevOpsTaskWorkItem: mocks.setTaskWorkItem,
    }),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
}));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: ({
    initialValues,
    onSuccess,
  }: {
    initialValues?: { description?: string };
    onSuccess?: (task: { id: string }) => void;
  }) => (
    <>
      <p data-testid="task-description">{initialValues?.description}</p>
      <button type="button" onClick={() => onSuccess?.({ id: "task-1" })}>
        Create task
      </button>
    </>
  ),
}));

import { AzureDevOpsTaskLauncher } from "./azure-devops-task-launcher";

const timestamp = "2026-07-18T00:00:00Z";
const workspaceId = "workspace-1";
const createTaskButtonName = "Create task";
const workflow: Workflow = {
  id: "workflow-1" as Workflow["id"],
  workspace_id: workspaceId as Workflow["workspace_id"],
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
  workspace_id: workspaceId as Repository["workspace_id"],
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
      workspaceId={workspaceId}
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

it("updates the cache and store after associating the created task", async () => {
  const linked = { id: "link-1" };
  mocks.associate.mockResolvedValue(linked);
  renderLauncher();

  fireEvent.click(screen.getByRole("button", { name: createTaskButtonName }));

  await waitFor(() => expect(mocks.cache).toHaveBeenCalledWith(workspaceId, "task-1", linked));
  expect(mocks.setTaskPullRequest).toHaveBeenCalledWith("task-1", linked);
  expect(mocks.push).toHaveBeenCalledWith("/tasks/task-1");
});

it("reports a pull request association failure", async () => {
  mocks.associate.mockRejectedValue(new Error("Azure association failed"));
  renderLauncher();

  fireEvent.click(screen.getByRole("button", { name: createTaskButtonName }));

  await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("Azure association failed"));
  expect(mocks.close).toHaveBeenCalled();
  expect(mocks.push).toHaveBeenCalledWith("/tasks/task-1");
});

it("associates and caches a created task for a work item", async () => {
  const linked = { id: "work-item-link" };
  mocks.associateWorkItem.mockResolvedValue(linked);
  render(
    <AzureDevOpsTaskLauncher
      workspaceId={workspaceId}
      workflows={[workflow]}
      steps={[step]}
      repositories={[repository]}
      payload={{
        kind: "work-item",
        item: {
          id: 73,
          revision: 1,
          title: "Investigate Azure item links",
          type: "Issue",
          state: "To Do",
          project: "project-1",
        },
      }}
      onClose={mocks.close}
    />,
  );

  fireEvent.click(screen.getByRole("button", { name: createTaskButtonName }));

  await waitFor(() =>
    expect(mocks.associateWorkItem).toHaveBeenCalledWith(workspaceId, "task-1", {
      projectId: "project-1",
      workItemId: 73,
    }),
  );
  expect(mocks.cacheWorkItem).toHaveBeenCalledWith(workspaceId, "task-1", linked);
  expect(mocks.setTaskWorkItem).toHaveBeenCalledWith("task-1", linked);
});

it("uses the generated description when an action prompt is blank", () => {
  render(
    <AzureDevOpsTaskLauncher
      workspaceId={workspaceId}
      workflows={[workflow]}
      steps={[step]}
      repositories={[repository]}
      payload={{
        kind: "work-item",
        item: {
          id: 73,
          revision: 1,
          title: "Investigate Azure item links",
          type: "Issue",
          state: "To Do",
          project: "project-1",
        },
        action: {
          id: "new",
          label: "New action",
          hint: "",
          icon: "sparkle",
          promptTemplate: "   ",
        },
      }}
      onClose={mocks.close}
    />,
  );

  expect(screen.getByTestId("task-description").textContent).toContain(
    "Azure DevOps work item: 73",
  );
  // The fallback is UI copy the user reads (and can edit) in the create dialog,
  // so it resolves through the catalog rather than a bare literal. The English
  // is unchanged.
  expect(screen.getByTestId("task-description").textContent).toContain("(no description)");
});

it("strips markup from a work-item description instead of using the fallback", () => {
  render(
    <AzureDevOpsTaskLauncher
      workspaceId={workspaceId}
      workflows={[workflow]}
      steps={[step]}
      repositories={[repository]}
      payload={{
        kind: "work-item",
        item: {
          id: 74,
          revision: 1,
          title: "Has a description",
          type: "Issue",
          state: "To Do",
          project: "project-1",
          description: "<div>Real <b>description</b></div>",
        },
      }}
      onClose={mocks.close}
    />,
  );

  const text = screen.getByTestId("task-description").textContent ?? "";
  expect(text).toContain("Real description");
  expect(text).not.toContain("(no description)");
});

it("reports a missing work-item project after creating the task", async () => {
  render(
    <AzureDevOpsTaskLauncher
      workspaceId={workspaceId}
      workflows={[workflow]}
      steps={[step]}
      repositories={[repository]}
      payload={{
        kind: "work-item",
        item: {
          id: 73,
          revision: 1,
          title: "Investigate Azure item links",
          type: "Issue",
          state: "To Do",
        },
      }}
      onClose={mocks.close}
    />,
  );

  fireEvent.click(screen.getByRole("button", { name: createTaskButtonName }));

  await waitFor(() =>
    expect(mocks.toastError).toHaveBeenCalledWith(
      "Failed to link Azure DevOps work item: project is missing.",
    ),
  );
  expect(mocks.associateWorkItem).not.toHaveBeenCalled();
});
