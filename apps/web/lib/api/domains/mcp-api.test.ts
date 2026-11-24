import { afterEach, expect, it, vi } from "vitest";
import { replaceMCPSelections } from "./mcp-api";

afterEach(() => vi.unstubAllGlobals());

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1
it.each([
  ["repository", { expected_definition_ids: [] }],
  ["task_session", { expected_revision: 0 }],
  ["task_session", { expected_revision: 2, excluded_definition_ids: [] }],
] satisfies Array<["repository" | "task_session", import("./mcp-api").MCPSelectionWriteOptions]>)(
  "serializes a known-empty %s condition and only explicit exclusions",
  async (scope, condition) => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ definition_ids: [] }), { status: 200 }));
    vi.stubGlobal("fetch", fetcher);
    await replaceMCPSelections(scope, "owner", "workspace", ["docs"], condition);
    const init = fetcher.mock.calls[0][1] as RequestInit;
    expect(JSON.parse(init.body as string)).toEqual({
      workspace_id: "workspace",
      definition_ids: ["docs"],
      ...condition,
    });
  },
);
