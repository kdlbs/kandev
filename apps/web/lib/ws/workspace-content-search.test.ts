import { describe, expect, it, vi } from "vitest";
import type { WebSocketClient } from "./client";
import { searchWorkspaceContent } from "./workspace-files";

describe("searchWorkspaceContent", () => {
  it("sends a repository-scoped result request for the active session", async () => {
    const response = {
      results: [
        {
          repository_name: "frontend",
          path: "src/app.tsx",
          line: 12,
          column: 8,
          preview: "const searchable = true",
          match_ranges: [{ start: 6, end: 16 }],
        },
      ],
    };
    const request = vi.fn().mockResolvedValue(response);
    const client = { request } as unknown as WebSocketClient;

    await expect(searchWorkspaceContent(client, "session-1", "searchable", 50)).resolves.toEqual(
      response,
    );
    expect(request).toHaveBeenCalledWith("workspace.content.search", {
      session_id: "session-1",
      query: "searchable",
      limit_per_repo: 50,
    });
  });
});
