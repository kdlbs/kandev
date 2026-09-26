import { describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { buildHostApi } from "./host-api";

const WORKSPACE_ID = "workspace-1";
const OTHER_WORKSPACE_ID = "workspace-2";
const WORKFLOW_ID = "workflow-1";
const PROVIDER_ID = "bitbucket";
const PROVIDER_SCOPE = "https://bitbucket.org";

function createContextStore() {
  const store = createAppStore();
  store.setState((state) => ({
    ...state,
    workspaces: { ...state.workspaces, activeId: WORKSPACE_ID },
    workflows: {
      activeId: WORKFLOW_ID,
      items: [
        { id: WORKFLOW_ID, workspaceId: WORKSPACE_ID, name: "Default" },
        { id: "workflow-2", workspaceId: OTHER_WORKSPACE_ID, name: "Other" },
      ],
    },
    kanban: {
      ...state.kanban,
      workflowId: WORKFLOW_ID,
      steps: [
        { id: "step-2", title: "Doing", color: "blue", position: 2 },
        { id: "step-1", title: "Todo", color: "gray", position: 1 },
      ],
    },
    repositories: {
      ...state.repositories,
      itemsByWorkspaceId: {
        [WORKSPACE_ID]: [
          {
            id: "repository-1",
            workspace_id: WORKSPACE_ID,
            name: "app",
            source_type: "provider",
            local_path: "",
            provider: PROVIDER_ID,
            provider_repo_id: "repo-uuid",
            provider_scope: PROVIDER_SCOPE,
            provider_owner: "team",
            provider_name: "app",
            remote_url: "https://bitbucket.org/team/app.git",
            default_branch: "main",
            worktree_branch_prefix: "kandev/",
            pull_before_worktree: true,
            setup_script: "",
            cleanup_script: "",
            dev_script: "",
            copy_files: "",
            created_at: "2026-08-12T00:00:00Z",
            updated_at: "2026-08-12T00:00:00Z",
          },
        ],
      },
    },
  }));
  return store;
}

describe("plugin host context", () => {
  it("exposes all workspace ids and updates them when the workspace list changes", () => {
    const store = createAppStore();
    store.setState((state) => ({
      ...state,
      workspaces: {
        ...state.workspaces,
        items: [
          { id: WORKSPACE_ID, name: "Main" },
          { id: OTHER_WORKSPACE_ID, name: "Other" },
        ] as never,
      },
    }));
    const host = buildHostApi(PROVIDER_ID, store);
    const context = host.context as unknown as {
      getWorkspaceIds(): readonly string[];
      subscribeWorkspaces(listener: (workspaceIds: readonly string[]) => void): () => void;
    };
    const listener = vi.fn();
    const unsubscribe = context.subscribeWorkspaces(listener);

    expect(context.getWorkspaceIds()).toEqual([WORKSPACE_ID, OTHER_WORKSPACE_ID]);

    store.setState((state) => ({
      ...state,
      workspaces: {
        ...state.workspaces,
        items: [{ id: WORKSPACE_ID, name: "Main" }] as never,
      },
    }));

    expect(listener).toHaveBeenCalledExactlyOnceWith([WORKSPACE_ID]);
    unsubscribe();
  });

  it("exposes provider-neutral workspace and task-creation context", () => {
    const host = buildHostApi(PROVIDER_ID, createContextStore());

    expect(host.context.getActiveWorkspaceId()).toBe(WORKSPACE_ID);
    expect(host.context.getTaskCreationContext(WORKSPACE_ID)).toEqual({
      workspaceId: WORKSPACE_ID,
      workflowId: WORKFLOW_ID,
      defaultStepId: "step-1",
      steps: [
        { id: "step-1", title: "Todo" },
        { id: "step-2", title: "Doing" },
      ],
      repositories: [
        expect.objectContaining({
          id: "repository-1",
          provider: PROVIDER_ID,
          provider_repo_id: "repo-uuid",
          provider_scope: PROVIDER_SCOPE,
        }),
      ],
    });
    expect(
      host.context.resolveRepositoryId({
        workspaceId: WORKSPACE_ID,
        providerId: PROVIDER_ID,
        providerScope: PROVIDER_SCOPE,
        providerRepositoryId: "repo-uuid",
      }),
    ).toBe("repository-1");
  });

  it("subscribes to derived context changes without exposing the app store", () => {
    const store = createAppStore();
    const host = buildHostApi(PROVIDER_ID, store);
    const listener = vi.fn();
    const unsubscribe = host.context.subscribeActiveWorkspace(listener);

    store.setState((state) => ({
      ...state,
      workspaces: { ...state.workspaces, activeId: WORKSPACE_ID },
    }));
    store.setState((state) => ({ ...state, tasks: { ...state.tasks } }));
    unsubscribe();
    store.setState((state) => ({
      ...state,
      workspaces: { ...state.workspaces, activeId: OTHER_WORKSPACE_ID },
    }));

    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener).toHaveBeenCalledWith(WORKSPACE_ID);
  });
});

describe("plugin host context edge cases", () => {
  it("reads task-creation steps from a non-active workflow snapshot", () => {
    const store = createContextStore();
    store.setState((state) => ({
      ...state,
      kanbanMulti: {
        ...state.kanbanMulti,
        snapshots: {
          ...state.kanbanMulti.snapshots,
          "workflow-2": {
            workflowId: "workflow-2",
            workflowName: "Other",
            steps: [{ id: "other-step", title: "Backlog", color: "gray", position: 0 }],
            tasks: [],
          },
        },
      },
    }));

    expect(
      buildHostApi(PROVIDER_ID, store).context.getTaskCreationContext(OTHER_WORKSPACE_ID),
    ).toEqual({
      workspaceId: OTHER_WORKSPACE_ID,
      workflowId: "workflow-2",
      defaultStepId: "other-step",
      steps: [{ id: "other-step", title: "Backlog" }],
      repositories: [],
    });
  });

  it("fails closed when persisted provider identity is ambiguous", () => {
    const store = createContextStore();
    store.setState((state) => {
      const [repository] = state.repositories.itemsByWorkspaceId[WORKSPACE_ID] ?? [];
      if (!repository) return state;
      return {
        ...state,
        repositories: {
          ...state.repositories,
          itemsByWorkspaceId: {
            ...state.repositories.itemsByWorkspaceId,
            [WORKSPACE_ID]: [repository, { ...repository, id: "repository-duplicate" }],
          },
        },
      };
    });

    expect(
      buildHostApi(PROVIDER_ID, store).context.resolveRepositoryId({
        workspaceId: WORKSPACE_ID,
        providerId: PROVIDER_ID,
        providerScope: PROVIDER_SCOPE,
        providerRepositoryId: "repo-uuid",
      }),
    ).toBeUndefined();
  });
});
