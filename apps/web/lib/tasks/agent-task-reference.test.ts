import { describe, expect, it } from "vitest";
import { agentTaskReferenceId, agentTaskReferenceLabel } from "./agent-task-reference";

// @covers AC-UI-AGENT-TASK-REFERENCE-001.5, AC-UI-AGENT-TASK-REFERENCE-001.6
describe("agent task reference destinations", () => {
  it("accepts exact task links without resolving a title", () => {
    expect(agentTaskReferenceId("/t/task_123-abc")).toBe("task_123-abc");
    expect(agentTaskReferenceId(`/t/${"a".repeat(256)}`)).toHaveLength(256);
  });
  it.each([
    undefined,
    "",
    "/t/",
    "/tasks/abc",
    "https://example.com/t/abc",
    "//host/t/abc",
    "/t/abc/",
    "/t/abc?sessionId=xyz",
    "/t/abc#hash",
    "/t/a%2Fb",
    "/t/%61bc",
    "/t/..",
    "/t/a b",
    "/t/abc\n",
    `/t/${"a".repeat(257)}`,
    "abc",
  ])("rejects lookalike destination %s", (href) => {
    expect(agentTaskReferenceId(href)).toBeNull();
  });
});

// @covers AC-UI-AGENT-TASK-REFERENCE-001.1, AC-UI-AGENT-TASK-REFERENCE-001.6
it("accepts text with renderer motion spans but rejects authored rich labels", () => {
  expect(
    agentTaskReferenceLabel([
      { type: "text", value: "Fix " },
      {
        type: "element",
        tagName: "span",
        properties: { "data-chat-text-motion": 123 },
        children: [{ type: "text", value: "retry" }],
      },
    ]),
  ).toBe("Fix retry");
  expect(
    agentTaskReferenceLabel([
      { type: "element", tagName: "strong", children: [{ type: "text", value: "Fix" }] },
    ]),
  ).toBeNull();
  expect(agentTaskReferenceLabel([{ type: "text", value: " " }])).toBeNull();
});

// @covers AC-UI-AGENT-TASK-REFERENCE-001.6
it("preserves whitespace-only animation runs inside an otherwise valid label", () => {
  expect(
    agentTaskReferenceLabel([
      { type: "text", value: "Fix" },
      {
        type: "element",
        tagName: "span",
        properties: { "data-chat-text-motion": 123 },
        children: [{ type: "text", value: " " }],
      },
      { type: "text", value: "retry" },
    ]),
  ).toBe("Fix retry");
});
