import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createAgentProject,
  deleteAgentProject,
  listAgentProjects,
  readAgentProjectContextFile,
  updateAgentProject,
  writeAgentProjectContextFile,
} from "./agent-projects-api";

const API_BASE_URL = "http://kandev.test";

describe("agent projects API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("uses the workspace collection and archived query", async () => {
    const fetchMock = vi.fn().mockResolvedValue(Response.json({ projects: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listAgentProjects("workspace / one", true, { baseUrl: API_BASE_URL });

    expect(fetchMock).toHaveBeenCalledWith(
      `${API_BASE_URL}/api/v1/workspaces/workspace%20%2F%20one/agent-projects?archived=true`,
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("serializes project creation fields in the backend wire format", async () => {
    const fetchMock = vi.fn().mockResolvedValue(Response.json({ id: "project-1" }));
    vi.stubGlobal("fetch", fetchMock);

    await createAgentProject(
      "ws-1",
      {
        name: "Project",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "profile-1",
        economyProfileId: "profile-2",
        frontierProfileId: "profile-3",
        requestKey: "request-1",
      },
      { baseUrl: API_BASE_URL },
    );

    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual({
      name: "Project",
      repository_ids: ["repo-1"],
      primary_repository_id: "repo-1",
      coordinator_profile_id: "profile-1",
      economy_profile_id: "profile-2",
      frontier_profile_id: "profile-3",
      request_key: "request-1",
    });
  });

  it("explicitly applies the current workspace executor when repairing a project", async () => {
    const fetchMock = vi.fn().mockResolvedValue(Response.json({ id: "project-1" }));
    vi.stubGlobal("fetch", fetchMock);

    await updateAgentProject(
      "ws-1",
      "project-1",
      { revision: 3, applyWorkspaceDefaultExecutor: true },
      { baseUrl: API_BASE_URL },
    );

    expect(JSON.parse(fetchMock.mock.calls[0][1].body as string)).toEqual({
      revision: 3,
      apply_workspace_default_executor: true,
    });
  });

  it("encodes context paths and sends the optimistic write hash", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(Response.json({ path: "notes.md", content: "notes", hash: "v1" }))
      .mockResolvedValueOnce(Response.json({ path: "notes.md", hash: "v2" }));
    vi.stubGlobal("fetch", fetchMock);

    await readAgentProjectContextFile("ws-1", "project-1", "folder/my notes.md", {
      baseUrl: API_BASE_URL,
    });
    await writeAgentProjectContextFile(
      "ws-1",
      "project-1",
      { path: "folder/my notes.md", content: "changed", expectedHash: "v1" },
      { baseUrl: API_BASE_URL },
    );

    expect(fetchMock.mock.calls[0][0]).toContain("path=folder%2Fmy+notes.md");
    expect(JSON.parse(fetchMock.mock.calls[1][1].body as string)).toEqual({
      path: "folder/my notes.md",
      content: "changed",
      expected_hash: "v1",
    });
  });

  it("sends the explicit context deletion choice", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await deleteAgentProject("ws-1", "project-1", true, true, { baseUrl: API_BASE_URL });

    expect(fetchMock).toHaveBeenCalledWith(
      `${API_BASE_URL}/api/v1/workspaces/ws-1/agent-projects/project-1?delete_context=true&discard_worktree_changes=true`,
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});
