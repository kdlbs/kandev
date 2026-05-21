import { describe, expect, it } from "vitest";
import { extractKandevStem, extractMcpResult, shortId } from "./parse";

describe("extractKandevStem", () => {
  it("strips the mcp__kandev__ namespace and the _kandev suffix", () => {
    expect(extractKandevStem("mcp__kandev__list_tasks_kandev")).toBe("list_tasks");
  });

  it("handles the codex-style kandev/ prefix", () => {
    expect(extractKandevStem("kandev/list_tasks_kandev")).toBe("list_tasks");
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
