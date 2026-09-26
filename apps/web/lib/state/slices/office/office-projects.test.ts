import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice, Project } from "./types";

const WS = "ws-1";
const OTHER_WS = "ws-2";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

function projectsOf(store: ReturnType<typeof makeStore>, workspaceId: string): Project[] {
  return store.getState().office.projectsByWorkspaceId[workspaceId] ?? [];
}

function makeProject(id: string, name: string, workspace = WS): Project {
  return {
    id,
    workspaceId: workspace,
    name,
    status: "active",
    color: "#3b82f6",
    repositories: ["https://github.com/team/backend"],
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

describe("project store actions", () => {
  it("setProjects replaces the list", () => {
    const store = makeStore();
    const projects = [makeProject("p1", "Project 1"), makeProject("p2", "Project 2")];
    store.getState().setProjects(WS, projects);
    expect(projectsOf(store, WS)).toHaveLength(2);
  });

  it("addProject appends to the list", () => {
    const store = makeStore();
    store.getState().setProjects(WS, [makeProject("p1", "Project 1")]);
    store.getState().addProject(WS, makeProject("p2", "Project 2"));
    expect(projectsOf(store, WS)).toHaveLength(2);
    expect(projectsOf(store, WS)[1].name).toBe("Project 2");
  });

  it("addProject seeds a workspace that has no list yet", () => {
    const store = makeStore();
    store.getState().addProject(WS, makeProject("p1", "Project 1"));
    expect(projectsOf(store, WS)).toHaveLength(1);
  });

  it("updateProject patches an existing project", () => {
    const store = makeStore();
    store.getState().setProjects(WS, [makeProject("p1", "Project 1")]);
    store.getState().updateProject(WS, "p1", { name: "Updated", status: "completed" });
    const project = projectsOf(store, WS)[0];
    expect(project.name).toBe("Updated");
    expect(project.status).toBe("completed");
  });

  it("updateProject is no-op for missing id", () => {
    const store = makeStore();
    store.getState().setProjects(WS, [makeProject("p1", "Project 1")]);
    store.getState().updateProject(WS, "missing", { name: "Nope" });
    expect(projectsOf(store, WS)[0].name).toBe("Project 1");
  });

  it("updateProject is no-op for a workspace with no list", () => {
    const store = makeStore();
    store.getState().setProjects(WS, [makeProject("p1", "Project 1")]);
    store.getState().updateProject(OTHER_WS, "p1", { name: "Nope" });
    expect(projectsOf(store, WS)[0].name).toBe("Project 1");
  });

  it("removeProject removes by id", () => {
    const store = makeStore();
    store
      .getState()
      .setProjects(WS, [makeProject("p1", "Project 1"), makeProject("p2", "Project 2")]);
    store.getState().removeProject(WS, "p1");
    expect(projectsOf(store, WS)).toHaveLength(1);
    expect(projectsOf(store, WS)[0].id).toBe("p2");
  });

  it("removeProject is no-op for missing id", () => {
    const store = makeStore();
    store.getState().setProjects(WS, [makeProject("p1", "Project 1")]);
    store.getState().removeProject(WS, "missing");
    expect(projectsOf(store, WS)).toHaveLength(1);
  });

  it("setProjects overwrites that workspace's projects only", () => {
    const store = makeStore();
    store.getState().setProjects(WS, [makeProject("p1", "Old")]);
    store.getState().setProjects(OTHER_WS, [makeProject("q1", "Other", OTHER_WS)]);
    store.getState().setProjects(WS, [makeProject("p2", "New")]);

    expect(projectsOf(store, WS)).toHaveLength(1);
    expect(projectsOf(store, WS)[0].name).toBe("New");
    expect(projectsOf(store, OTHER_WS)[0].name).toBe("Other");
  });
});
