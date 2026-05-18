import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice, Project } from "./types";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

function makeProject(id: string, name: string): Project {
  return {
    id,
    workspaceId: "ws-1",
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
    store.getState().setProjects(projects);
    expect(store.getState().office.projects).toHaveLength(2);
  });

  it("addProject appends to the list", () => {
    const store = makeStore();
    store.getState().setProjects([makeProject("p1", "Project 1")]);
    store.getState().addProject(makeProject("p2", "Project 2"));
    expect(store.getState().office.projects).toHaveLength(2);
    expect(store.getState().office.projects[1].name).toBe("Project 2");
  });

  it("updateProject patches an existing project", () => {
    const store = makeStore();
    store.getState().setProjects([makeProject("p1", "Project 1")]);
    store.getState().updateProject("p1", { name: "Updated", status: "completed" });
    const project = store.getState().office.projects[0];
    expect(project.name).toBe("Updated");
    expect(project.status).toBe("completed");
  });

  it("updateProject is no-op for missing id", () => {
    const store = makeStore();
    store.getState().setProjects([makeProject("p1", "Project 1")]);
    store.getState().updateProject("missing", { name: "Nope" });
    expect(store.getState().office.projects[0].name).toBe("Project 1");
  });

  it("removeProject removes by id", () => {
    const store = makeStore();
    store.getState().setProjects([makeProject("p1", "Project 1"), makeProject("p2", "Project 2")]);
    store.getState().removeProject("p1");
    expect(store.getState().office.projects).toHaveLength(1);
    expect(store.getState().office.projects[0].id).toBe("p2");
  });

  it("removeProject is no-op for missing id", () => {
    const store = makeStore();
    store.getState().setProjects([makeProject("p1", "Project 1")]);
    store.getState().removeProject("missing");
    expect(store.getState().office.projects).toHaveLength(1);
  });

  it("setProjects overwrites existing projects", () => {
    const store = makeStore();
    store.getState().setProjects([makeProject("p1", "Old")]);
    store.getState().setProjects([makeProject("p2", "New")]);
    expect(store.getState().office.projects).toHaveLength(1);
    expect(store.getState().office.projects[0].name).toBe("New");
  });
});
