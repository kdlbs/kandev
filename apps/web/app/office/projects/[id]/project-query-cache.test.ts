import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { Project } from "@/lib/state/slices/office/types";
import {
  readProjectFromListCache,
  removeProjectFromList,
  upsertProjectInList,
} from "./project-query-cache";

const TIMESTAMP = "2026-06-30T00:00:00Z";

function project(overrides: Partial<Project> = {}): Project {
  return {
    id: "project-1",
    workspaceId: "workspace-1",
    name: "Project",
    status: "active",
    createdAt: TIMESTAMP,
    updatedAt: TIMESTAMP,
    ...overrides,
  };
}

describe("project query cache helpers", () => {
  it("does not create a partial project list when upserting into a missing list", () => {
    expect(upsertProjectInList(undefined, project())).toBeUndefined();
  });

  it("patches an existing project list", () => {
    const updated = project({ name: "Updated" });

    expect(
      upsertProjectInList({ projects: [project(), project({ id: "project-2" })] }, updated),
    ).toEqual({
      projects: [updated, expect.objectContaining({ id: "project-2" })],
    });
  });

  it("does not create an empty project list when deleting from a missing list", () => {
    expect(removeProjectFromList(undefined, "project-1")).toBeUndefined();
  });

  it("removes a project from an existing project list", () => {
    expect(
      removeProjectFromList({ projects: [project(), project({ id: "project-2" })] }, "project-1"),
    ).toEqual({
      projects: [expect.objectContaining({ id: "project-2" })],
    });
  });

  it("reads a project from the cached workspace project list", () => {
    const queryClient = new QueryClient();
    const cachedProject = project({ id: "project-2", name: "Cached Project" });
    queryClient.setQueryData(qk.office.projects("workspace-1"), {
      projects: [project(), cachedProject],
    });

    expect(readProjectFromListCache(queryClient, "workspace-1", "project-2")).toBe(cachedProject);
    expect(readProjectFromListCache(queryClient, "workspace-2", "project-2")).toBeNull();
  });
});
