import { describe, expect, it, vi } from "vitest";
import type { WebSocketClient } from "./client";
import {
  requestFileContent,
  requestFileContentAtRef,
  searchWorkspaceFiles,
} from "./workspace-files";
import * as workspaceFiles from "./workspace-files";

function clientReturning(response: Record<string, unknown>) {
  return {
    request: vi.fn().mockResolvedValue(response),
  } as unknown as WebSocketClient;
}

describe("workspace file content", () => {
  function textResponseClient() {
    return clientReturning({
      path: "src/Main.kt",
      content: "fun main() = Unit",
      size: 17,
    });
  }

  it("normalizes omitted text classification for current content", async () => {
    const response = await requestFileContent(textResponseClient(), "session-1", "src/Main.kt");

    expect(response.is_binary).toBe(false);
  });

  it("normalizes omitted text classification for content at a ref", async () => {
    const response = await requestFileContentAtRef(
      textResponseClient(),
      "session-1",
      "src/Main.kt",
      "HEAD~1",
    );

    expect(response.is_binary).toBe(false);
  });

  it("preserves an explicit binary classification", async () => {
    const client = clientReturning({
      path: "image.bin",
      content: "AAE=",
      size: 2,
      is_binary: true,
    });

    await expect(requestFileContent(client, "session-1", "image.bin")).resolves.toMatchObject({
      is_binary: true,
    });
  });
});

it("preserves file-search defaults for callers without cancellation", async () => {
  const client = clientReturning({ files: ["src/app.ts"] });
  await expect(searchWorkspaceFiles(client, "session-1", "app")).resolves.toEqual({
    files: ["src/app.ts"],
  });
  expect(client.request).toHaveBeenCalledWith("workspace.files.search", {
    session_id: "session-1",
    query: "app",
    limit: 20,
  });
});

it("lists root files with the session-scoped workspace action", async () => {
  const response = {
    supported: true,
    files: [{ path: "dashboard.json", size: 12, modified_at: "2026-11-19T10:00:00Z" }],
    truncated: false,
    destinations: [{ repository_name: "web", path: "apps/web" }],
  };
  const client = clientReturning(response);
  const listWorkspaceRootFiles = (workspaceFiles as unknown as Record<string, unknown>)[
    "listWorkspaceRootFiles"
  ];

  expect(listWorkspaceRootFiles).toBeTypeOf("function");
  await expect(
    (listWorkspaceRootFiles as (client: WebSocketClient, sessionId: string) => Promise<unknown>)(
      client,
      "session-1",
    ),
  ).resolves.toEqual(response);
  expect(client.request).toHaveBeenCalledWith("workspace.root_files.list", {
    session_id: "session-1",
  });
});
