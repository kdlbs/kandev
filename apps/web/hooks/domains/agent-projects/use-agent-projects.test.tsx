import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { selectAgentProjects } from "@/lib/state/slices/agent-projects/selectors";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { publishAgentProjectTaskEvent } from "@/lib/ws/handlers/agent-project-events";
import {
  loadAgentProjects,
  useAgentProjectMutations,
  useAgentProjects,
} from "./use-agent-projects";

const mocks = vi.hoisted(() => ({ list: vi.fn(), remove: vi.fn() }));

vi.mock("@/lib/api/domains/agent-projects-api", () => ({
  archiveAgentProject: vi.fn(),
  createAgentProject: vi.fn(),
  deleteAgentProject: mocks.remove,
  listAgentProjects: mocks.list,
  restoreAgentProject: vi.fn(),
  updateAgentProject: vi.fn(),
}));

const project: AgentProject = {
  id: "project-1",
  workspace_id: "workspace-1",
  name: "Project",
  repository_ids: ["repo-1"],
  primary_repository_id: "repo-1",
  coordinator_profile_id: "coordinator",
  economy_profile_id: "economy",
  frontier_profile_id: "frontier",
  executor_profile_id: "executor",
  main_task_id: "coordinator-task",
  revision: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  tasks: [],
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.resetAllMocks();
});

async function assertQueuedRefreshSettles(workspaceId: string) {
  const initial = deferred<{ projects: AgentProject[] }>();
  const competing = deferred<{ projects: AgentProject[] }>();
  const queued = deferred<{ projects: AgentProject[] }>();
  mocks.list
    .mockReturnValueOnce(initial.promise)
    .mockReturnValueOnce(competing.promise)
    .mockReturnValueOnce(queued.promise);

  const { result } = renderHook(() => useAppStoreApi(), {
    wrapper: ({ children }) => createElement(StateProvider, null, children),
  });
  const initialLoad = loadAgentProjects(result.current, workspaceId);
  await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));

  const competingLoad = initialLoad.then(() =>
    loadAgentProjects(result.current, workspaceId, false, true),
  );
  const queuedLoad = loadAgentProjects(result.current, workspaceId, false, true);
  let competingSettled = false;
  let queuedSettled = false;
  void competingLoad.then(() => {
    competingSettled = true;
  });
  void queuedLoad.then(() => {
    queuedSettled = true;
  });

  await act(async () => initial.resolve({ projects: [project] }));
  await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(2));
  expect(competingSettled).toBe(false);
  expect(queuedSettled).toBe(false);

  await act(async () => competing.resolve({ projects: [project] }));
  await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(3));
  expect(competingSettled).toBe(true);
  expect(queuedSettled).toBe(false);

  await act(async () => queued.resolve({ projects: [project] }));
  await expect(competingLoad).resolves.toBeUndefined();
  await expect(queuedLoad).resolves.toBeUndefined();
  expect(queuedSettled).toBe(true);
}

describe("useAgentProjects live worker updates", () => {
  it("settles a queued refresh when a competing forced refresh starts first", async () => {
    await assertQueuedRefreshSettles(project.workspace_id);
  });

  it("loads worker creation and state changes from project task events", async () => {
    const workspaceId = project.workspace_id;
    let resolveInitial!: (response: { projects: AgentProject[] }) => void;
    mocks.list
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveInitial = resolve;
          }),
      )
      .mockResolvedValueOnce({
        projects: [
          {
            ...project,
            tasks: [
              {
                id: "worker-1",
                title: "Worker",
                state: "RUNNING",
                parent_id: "coordinator-task",
                updated_at: "2026-01-01T00:01:00Z",
              },
            ],
          },
        ],
      })
      .mockResolvedValueOnce({
        projects: [
          {
            ...project,
            tasks: [
              {
                id: "worker-1",
                title: "Worker",
                state: "DONE",
                parent_id: "coordinator-task",
                updated_at: "2026-01-01T00:02:00Z",
              },
            ],
          },
        ],
      });

    const { result } = renderHook(() => useAgentProjects(workspaceId), {
      wrapper: ({ children }) => createElement(StateProvider, null, children),
    });
    await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));

    act(() =>
      publishAgentProjectTaskEvent({ workspace_id: workspaceId, agent_project_id: "project-1" }),
    );
    await act(async () => resolveInitial({ projects: [project] }));
    await waitFor(() => expect(result.current.projects[0]?.tasks[0]?.state).toBe("RUNNING"));
    expect(mocks.list).toHaveBeenCalledTimes(2);

    act(() =>
      publishAgentProjectTaskEvent({ workspace_id: workspaceId, agent_project_id: "project-1" }),
    );
    await waitFor(() => expect(result.current.projects[0]?.tasks[0]?.state).toBe("DONE"));
    expect(mocks.list).toHaveBeenCalledTimes(3);
  });

  it("removes a deleted project immediately while list refresh is pending", async () => {
    const workspaceId = project.workspace_id;
    mocks.remove.mockResolvedValue(undefined);
    mocks.list.mockImplementation(() => new Promise(() => {}));

    const { result } = renderHook(
      () => ({ mutations: useAgentProjectMutations(), store: useAppStoreApi() }),
      { wrapper: ({ children }) => createElement(StateProvider, null, children) },
    );
    act(() => {
      result.current.store.getState().setAgentProjects(workspaceId, [project]);
      result.current.store.getState().setAgentProjects(workspaceId, [project], true);
    });

    await act(async () => {
      await result.current.mutations.remove(workspaceId, project.id, false, false);
    });

    expect(mocks.remove).toHaveBeenCalledWith(workspaceId, project.id, false, false);
    expect(selectAgentProjects(result.current.store.getState(), workspaceId)).toEqual([]);
    expect(selectAgentProjects(result.current.store.getState(), workspaceId, true)).toEqual([]);
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });
});
