import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  createRepositorySet,
  deleteRepositorySet,
  listRepositorySets,
  updateRepositorySet,
} from "./workspace-api";

type RecordedCall = { url: string; init: RequestInit | undefined };

const SET_ITEM_PATH = "/api/v1/repository-sets/set-1";
const SET_ID = "set-1";
const REPO_WEB = "repo-web";
const REPO_GATEWAY = "repo-gateway";
const WORKSPACE_COLLECTION_PATH = "/api/v1/workspaces/ws-1/repository-sets";

let calls: RecordedCall[];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  calls = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init });
      if (init?.method === "DELETE") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(jsonResponse({ repository_sets: [], total: 0 }));
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function bodyOf(call: RecordedCall): Record<string, unknown> {
  return JSON.parse(String(call.init?.body ?? "{}"));
}

describe("repository set API client", () => {
  it("lists a workspace's sets from the workspace-scoped collection route", async () => {
    await listRepositorySets("ws-1");

    expect(calls).toHaveLength(1);
    expect(calls[0].url).toContain(WORKSPACE_COLLECTION_PATH);
  });

  it("encodes the workspace id into the path", async () => {
    await listRepositorySets("ws/1");

    expect(calls[0].url).toContain("/api/v1/workspaces/ws%2F1/repository-sets");
  });

  it("posts name, description, and ordered repository ids", async () => {
    await createRepositorySet("ws-1", {
      name: "Full-stack",
      description: "web + gateway",
      repositoryIds: [REPO_WEB, REPO_GATEWAY],
    });

    expect(calls[0].url).toContain(WORKSPACE_COLLECTION_PATH);
    expect(calls[0].init?.method).toBe("POST");
    expect(bodyOf(calls[0])).toEqual({
      name: "Full-stack",
      description: "web + gateway",
      repository_ids: [REPO_WEB, REPO_GATEWAY],
    });
  });

  it("posts ordered repository members with their saved bases", async () => {
    await createRepositorySet("ws-1", {
      name: "Release",
      repositories: [
        { repositoryId: REPO_GATEWAY, baseBranch: "develop" },
        { repositoryId: REPO_WEB },
      ],
    });

    expect(bodyOf(calls[0])).toEqual({
      name: "Release",
      description: "",
      repositories: [
        { repository_id: REPO_GATEWAY, base_branch: "develop" },
        { repository_id: REPO_WEB, base_branch: "" },
      ],
    });
  });

  it("patches the flat item route and omits fields the caller left out", async () => {
    await updateRepositorySet(SET_ID, { name: "Renamed" });

    expect(calls[0].url).toContain(SET_ITEM_PATH);
    expect(calls[0].init?.method).toBe("PATCH");
    // An absent repository_ids must stay absent: the backend reads a present
    // empty list as a rejected request, not as "leave membership alone".
    expect(bodyOf(calls[0])).toEqual({ name: "Renamed" });
  });

  it("sends an explicit repository_ids list when reordering or replacing members", async () => {
    await updateRepositorySet(SET_ID, { repositoryIds: [REPO_GATEWAY, REPO_WEB] });

    expect(bodyOf(calls[0])).toEqual({ repository_ids: [REPO_GATEWAY, REPO_WEB] });
  });

  it("sends an empty description when the caller clears it", async () => {
    await updateRepositorySet(SET_ID, { description: "" });

    expect(bodyOf(calls[0])).toEqual({ description: "" });
  });

  it("patches ordered repository members with their saved bases", async () => {
    await updateRepositorySet(SET_ID, {
      repositories: [{ repositoryId: REPO_WEB, baseBranch: "develop" }],
    });

    expect(bodyOf(calls[0])).toEqual({
      repositories: [{ repository_id: REPO_WEB, base_branch: "develop" }],
    });
  });

  it("deletes the flat item route", async () => {
    await deleteRepositorySet(SET_ID);

    expect(calls[0].url).toContain(SET_ITEM_PATH);
    expect(calls[0].init?.method).toBe("DELETE");
  });
});
