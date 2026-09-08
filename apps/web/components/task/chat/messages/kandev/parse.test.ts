import { describe, expect, it } from "vitest";
import { extractKandevArgs, extractKandevStem, extractMcpResult, shortId } from "./parse";

describe("extractKandevArgs", () => {
  it("unwraps arguments from the real Codex ACP MCP envelope", () => {
    const argumentsValue = { version: 1, title: "Service failures", blocks: [] };
    expect(
      extractKandevArgs({
        raw_input: {
          server: "kandev",
          tool: "show_rich_output_kandev",
          arguments: argumentsValue,
        },
      }),
    ).toEqual(argumentsValue);
  });
});

describe("extractKandevStem", () => {
  it("strips the mcp__kandev__ namespace and the _kandev suffix", () => {
    expect(extractKandevStem("mcp__kandev__list_tasks_kandev")).toBe("list_tasks");
  });

  it("handles the codex-style kandev/ prefix", () => {
    expect(extractKandevStem("kandev/list_tasks_kandev")).toBe("list_tasks");
  });

  it("handles the dotted Codex ACP title fallback", () => {
    expect(extractKandevStem("mcp.kandev.show_rich_output_kandev")).toBe("show_rich_output");
  });

  it("handles a bare suffix-only name", () => {
    expect(extractKandevStem("create_task_kandev")).toBe("create_task");
  });

  it("returns null for non-kandev tools", () => {
    expect(extractKandevStem("mcp__github__list_issues")).toBeNull();
    expect(extractKandevStem("Edit")).toBeNull();
    expect(extractKandevStem("")).toBeNull();
    expect(extractKandevStem(undefined)).toBeNull();
  });

  it("returns null when the suffix is bare (no stem)", () => {
    expect(extractKandevStem("_kandev")).toBeNull();
  });
});

describe("extractMcpResult", () => {
  it("parses a single MCP content block", () => {
    const blocks = [{ type: "text", text: '{"steps": [{"name": "Backlog"}]}' }];
    expect(extractMcpResult(blocks)).toEqual({ steps: [{ name: "Backlog" }] });
  });

  it("joins multiple text blocks before JSON parsing", () => {
    const blocks = [
      { type: "text", text: '{"a":' },
      { type: "text", text: "1}" },
    ];
    expect(extractMcpResult(blocks)).toEqual({ a: 1 });
  });

  it("returns the raw string if blocks contain non-JSON text", () => {
    const blocks = [{ type: "text", text: "hello world" }];
    expect(extractMcpResult(blocks)).toBe("hello world");
  });

  it("unwraps a string containing JSON", () => {
    expect(extractMcpResult('{"foo":"bar"}')).toEqual({ foo: "bar" });
  });

  it("returns the raw string for non-JSON strings", () => {
    expect(extractMcpResult("not json")).toBe("not json");
  });

  it("returns null for empty/missing values", () => {
    expect(extractMcpResult(undefined)).toBeNull();
    expect(extractMcpResult(null)).toBeNull();
    expect(extractMcpResult("")).toBeNull();
    expect(extractMcpResult("   ")).toBeNull();
  });

  it("unwraps a CallToolResult-style object with content[]", () => {
    const wrapped = { content: [{ type: "text", text: '{"ok":true}' }] };
    expect(extractMcpResult(wrapped)).toEqual({ ok: true });
  });

  it("returns plain objects untouched", () => {
    expect(extractMcpResult({ foo: 1 })).toEqual({ foo: 1 });
  });

  it("prefers standard MCP structuredContent over its text fallback", () => {
    expect(
      extractMcpResult({
        content: [{ type: "text", text: "fallback" }],
        structuredContent: { version: 1, resolved_charts: [] },
      }),
    ).toEqual({ version: 1, resolved_charts: [] });
  });

  it("falls back to text content when structuredContent is null", () => {
    expect(
      extractMcpResult({
        _meta: null,
        content: [{ type: "text", text: '{"total":2,"workspaces":[{"id":"w1"}]}' }],
        structuredContent: null,
      }),
    ).toEqual({ total: 2, workspaces: [{ id: "w1" }] });
  });

  it("falls back to text content when structured_content is null", () => {
    expect(
      extractMcpResult({
        content: [{ type: "text", text: '{"total":1}' }],
        structured_content: null,
      }),
    ).toEqual({ total: 1 });
  });

  it("unwraps the raw result wrapper emitted by ACP clients", () => {
    expect(extractMcpResult({ result: '{"version":1,"resolved_charts":[]}' })).toEqual({
      version: 1,
      resolved_charts: [],
    });
  });

  it("unwraps the real Codex ACP rawOutput result object", () => {
    const snapshot = { version: 1, resolved_charts: [] };
    expect(
      extractMcpResult({
        error: null,
        result: {
          content: [{ type: "text", text: JSON.stringify(snapshot) }],
          structuredContent: snapshot,
        },
      }),
    ).toEqual(snapshot);
  });
});

describe("shortId", () => {
  it("truncates long uuids with an ellipsis", () => {
    expect(shortId("4aad62c5-e549-495a-888b-14feecc28334")).toBe("4aad62c5…");
  });

  it("returns short ids unchanged", () => {
    expect(shortId("abc")).toBe("abc");
    expect(shortId("")).toBe("");
    expect(shortId(undefined)).toBe("");
  });
});
