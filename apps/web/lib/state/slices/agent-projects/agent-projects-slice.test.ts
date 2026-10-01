import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { selectAgentProjects } from "./selectors";
import type { AgentProject } from "@/lib/types/http-agent-projects";

const project = (id: string, workspaceId = "ws-1") =>
  ({
    id,
    workspace_id: workspaceId,
    name: id,
    repository_ids: [],
    primary_repository_id: "",
    coordinator_profile_id: "",
    economy_profile_id: "",
    frontier_profile_id: "",
    executor_profile_id: "",
    main_task_id: "",
    revision: 1,
    created_at: "",
    updated_at: "",
    tasks: [],
  }) satisfies AgentProject;

describe("agent project state", () => {
  it("keeps active and archived project lists isolated by workspace", () => {
    const store = createAppStore();

    store.getState().setAgentProjects("ws-1", [project("active")]);
    store.getState().setAgentProjects("ws-1", [project("archived")], true);
    store.getState().setAgentProjects("ws-2", [project("other")]);

    expect(selectAgentProjects(store.getState(), "ws-1").map((item) => item.id)).toEqual([
      "active",
    ]);
    expect(selectAgentProjects(store.getState(), "ws-1", true).map((item) => item.id)).toEqual([
      "archived",
    ]);
    expect(selectAgentProjects(store.getState(), "ws-2").map((item) => item.id)).toEqual(["other"]);
  });

  it("upserts a project and removes it from both views", () => {
    const store = createAppStore();
    store.getState().setAgentProjects("ws-1", [project("one")]);
    store.getState().upsertAgentProject("ws-1", { ...project("one"), name: "Updated" });
    store.getState().upsertAgentProject("ws-1", project("two"));

    expect(selectAgentProjects(store.getState(), "ws-1").map((item) => item.name)).toEqual([
      "Updated",
      "two",
    ]);

    store.getState().removeAgentProject("ws-1", "one");
    expect(selectAgentProjects(store.getState(), "ws-1").map((item) => item.id)).toEqual(["two"]);
  });
});
