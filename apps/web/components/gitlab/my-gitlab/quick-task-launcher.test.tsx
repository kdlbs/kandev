import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { Task } from "@/lib/types/http";
import type { GitLabLaunchPayload } from "./quick-task-launcher";

const createTaskMRMock = vi.fn();
const pushMock = vi.fn();
let dialogProps: Record<string, unknown> | null = null;
const WORKSPACE_ID = "ws-1";
const WORKFLOW_ID = "workflow-1";
const FIXED_DATE = "2026-01-01T00:00:00Z";
const PROJECT_NAMESPACE = "group/subgroup";
const PROJECT_PATH = `${PROJECT_NAMESPACE}/project`;

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  createTaskMR: (...args: unknown[]) => createTaskMRMock(...args),
}));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: pushMock }) }));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: (props: Record<string, unknown>) => {
    dialogProps = props;
    return null;
  },
}));

import { QuickTaskLauncher } from "./quick-task-launcher";

const preset = {
  id: "review",
  label: "Review",
  hint: "Review changes",
  icon: () => null,
  prompt: ({ url }: { url: string; title: string }) => `Review ${url}`,
};

const payload: GitLabLaunchPayload = {
  kind: "mr",
  preset,
  mr: {
    id: 1,
    iid: 7,
    project_id: 10,
    title: "Review this",
    url: "https://gitlab.acme/group/subgroup/project/-/merge_requests/7",
    web_url: "https://gitlab.acme/group/subgroup/project/-/merge_requests/7",
    state: "opened",
    head_branch: "feature",
    head_sha: "abc",
    base_branch: "main",
    author_username: "alice",
    project_namespace: PROJECT_NAMESPACE,
    project_path: PROJECT_PATH,
    body: "",
    draft: false,
    merge_status: "can_be_merged",
    has_conflicts: false,
    additions: 1,
    deletions: 0,
    reviewers: [],
    assignees: [],
    created_at: FIXED_DATE,
    updated_at: FIXED_DATE,
  },
};

describe("GitLab QuickTaskLauncher", () => {
  beforeEach(() => {
    dialogProps = null;
    createTaskMRMock.mockReset().mockResolvedValue({ id: "association-1" });
    pushMock.mockReset();
  });

  it("prefills the matching GitLab repository and links the created task", async () => {
    render(
      <StateProvider>
        <ToastProvider>
          <QuickTaskLauncher
            workspaceId={WORKSPACE_ID}
            configuredHost="https://gitlab.acme"
            workflows={[{ id: WORKFLOW_ID, workspace_id: WORKSPACE_ID, name: "Default" } as never]}
            steps={[{ id: "step-1", workflow_id: WORKFLOW_ID, name: "Todo", position: 0 } as never]}
            repositories={[
              {
                id: "repo-host-collision",
                workspace_id: WORKSPACE_ID,
                provider: "gitlab",
                provider_host: "https://gitlab.example.net",
                provider_owner: PROJECT_NAMESPACE,
                provider_name: "project",
                default_branch: "main",
              } as never,
              {
                id: "repo-project-collision",
                workspace_id: WORKSPACE_ID,
                provider: "gitlab",
                provider_host: "https://gitlab.acme",
                provider_owner: "other/group",
                provider_name: "project",
                default_branch: "main",
              } as never,
              {
                id: "repo-1",
                workspace_id: WORKSPACE_ID,
                provider: "gitlab",
                provider_host: "https://gitlab.acme/",
                provider_owner: PROJECT_NAMESPACE,
                provider_name: "project",
                default_branch: "main",
              } as never,
            ]}
            payload={payload}
            onClose={vi.fn()}
          />
        </ToastProvider>
      </StateProvider>,
    );

    expect(dialogProps?.initialValues).toMatchObject({
      title: "Review: Review this",
      repositoryId: "repo-1",
      branch: "feature",
      checkoutBranch: "feature",
    });

    await act(async () => {
      await (dialogProps?.onSuccess as (task: Task) => Promise<void>)({
        id: "task-1",
        repositories: [{ repository_id: "repo-1" }],
      } as Task);
    });
    expect(createTaskMRMock).toHaveBeenCalledWith(
      {
        task_id: "task-1",
        repository_id: "repo-1",
        mr_url: payload.mr.web_url,
      },
      WORKSPACE_ID,
    );
    expect(pushMock).toHaveBeenCalledWith("/t/task-1");
  });
});

describe("GitLab QuickTaskLauncher recovery", () => {
  beforeEach(() => {
    dialogProps = null;
    createTaskMRMock.mockReset().mockResolvedValue({ id: "association-1" });
    pushMock.mockReset();
  });

  it("does not persist a durable association for issue launches", async () => {
    const issuePayload: GitLabLaunchPayload = {
      kind: "issue",
      preset,
      issue: {
        id: 2,
        iid: 3,
        project_id: 10,
        title: "Fix bug",
        body: "",
        url: "https://gitlab.acme/group/project/-/issues/3",
        web_url: "https://gitlab.acme/group/project/-/issues/3",
        state: "opened",
        author_username: "alice",
        project_namespace: "group",
        project_path: "group/project",
        labels: [],
        assignees: [],
        created_at: FIXED_DATE,
        updated_at: FIXED_DATE,
      },
    };
    render(
      <StateProvider>
        <ToastProvider>
          <QuickTaskLauncher
            workspaceId={WORKSPACE_ID}
            configuredHost="https://gitlab.acme"
            workflows={[{ id: WORKFLOW_ID, workspace_id: WORKSPACE_ID, name: "Default" } as never]}
            steps={[{ id: "step-1", workflow_id: WORKFLOW_ID, name: "Todo", position: 0 } as never]}
            repositories={[]}
            payload={issuePayload}
            onClose={vi.fn()}
          />
        </ToastProvider>
      </StateProvider>,
    );

    await act(async () => {
      await (dialogProps?.onSuccess as (task: Task) => Promise<void>)({
        id: "task-issue",
        repositories: [],
      } as unknown as Task);
    });
    expect(createTaskMRMock).not.toHaveBeenCalled();
    expect(dialogProps?.initialValues).toMatchObject({
      description: expect.stringContaining(issuePayload.issue.web_url),
    });
  });

  it("keeps task recovery coherent and reports a partial failure when MR linking fails", async () => {
    createTaskMRMock.mockRejectedValueOnce(new Error("repository origin does not match"));
    render(
      <StateProvider>
        <ToastProvider>
          <QuickTaskLauncher
            workspaceId={WORKSPACE_ID}
            configuredHost="https://gitlab.acme"
            workflows={[{ id: WORKFLOW_ID, workspace_id: WORKSPACE_ID, name: "Default" } as never]}
            steps={[{ id: "step-1", workflow_id: WORKFLOW_ID, name: "Todo", position: 0 } as never]}
            repositories={[
              {
                id: "repo-1",
                workspace_id: WORKSPACE_ID,
                provider: "gitlab",
                provider_host: "https://gitlab.acme",
                provider_owner: PROJECT_NAMESPACE,
                provider_name: "project",
                default_branch: "main",
              } as never,
            ]}
            payload={payload}
            onClose={vi.fn()}
          />
        </ToastProvider>
      </StateProvider>,
    );

    await act(async () => {
      await (dialogProps?.onSuccess as (task: Task) => Promise<void>)({
        id: "task-partial",
        repositories: [{ repository_id: "repo-1" }],
      } as Task);
    });

    expect(pushMock).toHaveBeenCalledWith("/t/task-partial");
    expect(screen.getByText("Task created, but merge request was not linked")).toBeTruthy();
    expect(
      screen.getByText(/Open the task and use Link GitLab merge request to retry/),
    ).toBeTruthy();
  });
});
