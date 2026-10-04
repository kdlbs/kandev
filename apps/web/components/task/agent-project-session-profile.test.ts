import { describe, expect, it } from "vitest";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import {
  filterAgentProjectSessionProfiles,
  resolveAgentProjectSessionProfile,
} from "./agent-project-session-profile";

const ids = {
  project: "project-1",
  workspace: "workspace-1",
  coordinatorTask: "coordinator-task",
  workerTask: "worker-task",
  ordinaryTask: "ordinary-task",
  missingProject: "missing-project",
  otherTask: "another-task",
};
const profileIds = {
  coordinatorA: "coordinator-a",
  coordinatorB: "coordinator-b",
  workerPinned: "worker-pinned",
};
const timestamp = "2026-01-01T00:00:00Z";

const project: AgentProject = {
  id: ids.project,
  workspace_id: ids.workspace,
  name: "Project",
  repository_ids: ["repo-1"],
  primary_repository_id: "repo-1",
  coordinator_profile_id: profileIds.coordinatorA,
  economy_profile_id: "economy",
  frontier_profile_id: "frontier",
  executor_profile_id: "executor",
  main_task_id: ids.coordinatorTask,
  revision: 1,
  created_at: timestamp,
  updated_at: timestamp,
  tasks: [{ id: ids.workerTask, title: "Worker", state: "RUNNING", updated_at: timestamp }],
};

describe("Agent Project new-session profile selection", () => {
  it("keeps ordinary task profile selection unchanged after membership is loaded", () => {
    const policy = resolveAgentProjectSessionProfile({
      taskId: ids.ordinaryTask,
      task: { id: ids.ordinaryTask },
      projects: [project],
      projectsLoaded: true,
      currentSessionTaskId: ids.ordinaryTask,
      currentSessionProfileId: "profile-b",
    });

    expect(policy).toEqual({ kind: "ordinary" });
    expect(
      filterAgentProjectSessionProfiles([{ id: "profile-a" }, { id: "profile-b" }], policy),
    ).toEqual([{ id: "profile-a" }, { id: "profile-b" }]);
  });

  it("uses the current configured coordinator profile instead of the active session profile", () => {
    const policy = resolveAgentProjectSessionProfile({
      taskId: ids.coordinatorTask,
      task: {
        id: ids.coordinatorTask,
        agent_project_id: ids.project,
        agent_project_tier: "coordinator",
      },
      projects: [project],
      projectsLoaded: true,
      currentSessionTaskId: ids.coordinatorTask,
      currentSessionProfileId: profileIds.coordinatorB,
    });

    expect(policy).toEqual({ kind: "restricted", profileId: profileIds.coordinatorA });
    expect(
      filterAgentProjectSessionProfiles(
        [{ id: profileIds.coordinatorA }, { id: profileIds.coordinatorB }, { id: "other" }],
        policy,
      ),
    ).toEqual([{ id: profileIds.coordinatorA }]);
  });

  it("keeps a worker pinned to its same-task session profile", () => {
    const policy = resolveAgentProjectSessionProfile({
      taskId: ids.workerTask,
      task: { id: ids.workerTask, agent_project_id: ids.project, agent_project_tier: "economy" },
      projects: [project],
      projectsLoaded: true,
      currentSessionTaskId: ids.workerTask,
      currentSessionProfileId: profileIds.workerPinned,
    });

    expect(policy).toEqual({ kind: "restricted", profileId: profileIds.workerPinned });
    expect(
      filterAgentProjectSessionProfiles(
        [{ id: profileIds.workerPinned }, { id: "economy" }, { id: "other" }],
        policy,
      ),
    ).toEqual([{ id: profileIds.workerPinned }]);
  });

  it("uses the worker metadata pin ahead of project defaults and unrelated session state", () => {
    const policy = resolveAgentProjectSessionProfile({
      taskId: ids.workerTask,
      task: {
        id: ids.workerTask,
        agent_project_id: ids.project,
        agent_project_tier: "economy",
        metadata: { agent_project_profile_id: profileIds.workerPinned },
      },
      projects: [project],
      projectsLoaded: true,
      currentSessionTaskId: ids.otherTask,
      currentSessionProfileId: "unrelated-profile",
    });

    expect(policy).toEqual({ kind: "restricted", profileId: profileIds.workerPinned });
  });

  it("recognizes a coordinator by the project's main task when the task projection omits identity", () => {
    const policy = resolveAgentProjectSessionProfile({
      taskId: ids.coordinatorTask,
      task: { id: ids.coordinatorTask },
      projects: [project],
      projectsLoaded: true,
    });

    expect(policy).toEqual({ kind: "restricted", profileId: profileIds.coordinatorA });
  });
});

describe("Agent Project new-session fail-closed cases", () => {
  it("fails closed while project membership is still loading or references a missing project", () => {
    expect(
      resolveAgentProjectSessionProfile({
        taskId: ids.coordinatorTask,
        task: { id: ids.coordinatorTask, agent_project_id: ids.project },
        projects: [],
        projectsLoaded: false,
      }),
    ).toEqual({ kind: "unavailable" });

    expect(
      resolveAgentProjectSessionProfile({
        taskId: ids.coordinatorTask,
        task: { id: ids.coordinatorTask, agent_project_id: ids.missingProject },
        projects: [project],
        projectsLoaded: true,
      }),
    ).toEqual({ kind: "unavailable" });
  });

  it("fails closed when a worker pin is missing and no same-task session supplies it", () => {
    const unpinnedWorker = resolveAgentProjectSessionProfile({
      taskId: ids.workerTask,
      task: { id: ids.workerTask, agent_project_id: ids.project, agent_project_tier: "economy" },
      projects: [project],
      projectsLoaded: true,
      currentSessionTaskId: ids.otherTask,
      currentSessionProfileId: "unrelated-profile",
    });
    expect(unpinnedWorker).toEqual({ kind: "unavailable" });
    expect(
      filterAgentProjectSessionProfiles([{ id: "worker-pinned" }, { id: "other" }], unpinnedWorker),
    ).toEqual([]);
  });
});

describe("Agent Project new-session feature flag", () => {
  it("preserves ordinary tasks when Agent Projects are disabled and fails closed on project identity", () => {
    expect(
      resolveAgentProjectSessionProfile({
        taskId: ids.ordinaryTask,
        task: { id: ids.ordinaryTask },
        projects: [],
        projectsEnabled: false,
        projectsLoaded: false,
      }),
    ).toEqual({ kind: "ordinary" });

    expect(
      resolveAgentProjectSessionProfile({
        taskId: "project-task",
        task: { id: "project-task", metadata: { agent_project_id: ids.project } },
        projects: [],
        projectsEnabled: false,
        projectsLoaded: false,
      }),
    ).toEqual({ kind: "unavailable" });
  });
});
