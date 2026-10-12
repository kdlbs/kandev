import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { PromptMentionChip, splitMarkdownPromptMentionSegments } from "./prompt-mention-components";

const PROMPT = {
  id: "prompt-1",
  name: "daily",
  content: "Review the daily report",
  builtin: false,
  created_at: "2026-09-12T00:00:00Z",
  updated_at: "2026-09-12T00:00:00Z",
};
const INITIAL_PROMPT_STATE = {
  prompts: { items: [PROMPT], loaded: true, loading: false },
};
const EMPTY_PROMPT_STATE = {
  prompts: { items: [], loaded: true, loading: false },
};
const PROMPT_MENTION_TEST_ID = "custom-prompt-mention";
const PROMPT_MENTION_LABEL_TEST_ID = "custom-prompt-mention-label";

const touchState = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchState.enabled,
}));

afterEach(() => {
  cleanup();
  touchState.enabled = false;
});

const promptNames = ["daily"];

function promptValues(content: string) {
  return splitMarkdownPromptMentionSegments(content, promptNames)
    .filter((segment) => segment.kind === "prompt")
    .map((segment) => segment.value);
}

describe("splitMarkdownPromptMentionSegments round-two boundaries", () => {
  it("does not reinterpret a mismatched backtick run as a delimiter", () => {
    expect(promptValues("`foo`` @daily`")).toEqual([]);
    expect(promptValues("``` @daily")).toEqual([]);
  });

  it("treats backslashes literally while scanning an inline code span", () => {
    expect(promptValues("`foo @daily\\` tail")).toEqual([]);
  });

  it("recognizes aliases after a CRLF closing fence", () => {
    expect(promptValues("```\r\ncode\r\n```\r\n@daily")).toEqual(["@daily"]);
  });

  it("does not use a code-span bracket as an inline-link label", () => {
    expect(promptValues('`[`](/url "title @daily")')).toEqual(["@daily"]);
  });
  it("does not hide a title alias after reference-style link syntax", () => {
    expect(promptValues('[label][ref](url "title @daily")')).toEqual(["@daily"]);
  });

  it("rejects escaped whitespace in a bare link destination", () => {
    expect(promptValues('[label](url\\ bar "title @daily")')).toEqual(["@daily"]);
  });

  it("skips titles in adjacent valid inline links", () => {
    expect(promptValues('[] [label](url "title @daily")')).toEqual([]);
  });

  it("keeps formatted aliases adjacent to ordinary text as text", () => {
    expect(promptValues("x**@daily**")).toEqual([]);
    expect(promptValues("x_@daily_")).toEqual([]);
  });

  it("keeps the longest prompt name when names share a prefix", () => {
    expect(
      splitMarkdownPromptMentionSegments("@Daily Summary", ["Daily", "Daily Summary"]),
    ).toEqual([{ kind: "prompt", value: "@Daily Summary", name: "Daily Summary" }]);
  });

  it("recognizes aliases nested in Markdown formatting", () => {
    expect(promptValues("**_@daily_**")).toEqual(["@daily"]);
    expect(promptValues("[**@daily**](/url)")).toEqual(["@daily"]);
  });

  it("does not hide aliases after malformed link-like text", () => {
    expect(promptValues('[x]foo "title @daily")')).toEqual(["@daily"]);
  });

  it("recognizes aliases after CR-only fenced code", () => {
    expect(promptValues("~~~\rcode\r~~~\r@daily")).toEqual(["@daily"]);
  });

  it("recognizes aliases in link labels adjacent to ordinary text", () => {
    expect(promptValues("x[@daily](url)")).toEqual(["@daily"]);
  });

  it("recognizes aliases at the start of formatted text", () => {
    expect(promptValues("**@daily now**")).toEqual(["@daily"]);
    expect(promptValues("[@daily label](/url)")).toEqual(["@daily"]);
  });

  it("does not close formatted aliases on escaped delimiters", () => {
    expect(promptValues("**@daily\\*")).toEqual([]);
    expect(promptValues("[@daily\\]")).toEqual([]);
  });
  it("recognizes aliases in supported long messages", () => {
    const content = `${"x".repeat(32_769)} @daily`;

    expect(splitMarkdownPromptMentionSegments(content, promptNames)).toEqual([
      { kind: "text", value: `${"x".repeat(32_769)} ` },
      { kind: "prompt", value: "@daily", name: "daily" },
    ]);
  });

  it("fails closed for oversized malformed Markdown mention input", () => {
    const content = `[${"x".repeat(40_000)} [@daily`;

    expect(splitMarkdownPromptMentionSegments(content, promptNames)).toEqual([
      { kind: "text", value: content },
    ]);
  });
});

describe("PromptMentionChip presentation", () => {
  it("keeps the default read-only chip presentation unchanged", () => {
    render(
      <StateProvider initialState={INITIAL_PROMPT_STATE}>
        <PromptMentionChip name={PROMPT.name} value="@daily" />
      </StateProvider>,
    );

    const chip = screen.getByTestId(PROMPT_MENTION_TEST_ID);
    expect(chip.className).toContain("border-emerald-300/35");
    expect(chip.className).toContain("bg-emerald-400/20");
    expect(chip.className).toContain("px-1.5");
    expect(chip.className).not.toContain("h-11");
    expect(chip.className).not.toContain("min-w-11");
  });

  it("keeps the default touch preview target at its established size", () => {
    touchState.enabled = true;
    render(
      <StateProvider initialState={INITIAL_PROMPT_STATE}>
        <PromptMentionChip name={PROMPT.name} value="@daily" />
      </StateProvider>,
    );

    const chip = screen.getByTestId(PROMPT_MENTION_TEST_ID);
    expect(chip.tagName).toBe("BUTTON");
    expect(chip.className).toContain("h-11");
    expect(chip.className).toContain("min-w-11");
  });

  it("uses the compact editable presentation only when opted in", () => {
    render(
      <StateProvider initialState={INITIAL_PROMPT_STATE}>
        <PromptMentionChip name={PROMPT.name} value="@daily" presentation="editable" />
      </StateProvider>,
    );

    const chip = screen.getByTestId(PROMPT_MENTION_TEST_ID);
    expect(chip.className).not.toContain("border-emerald-300/35");
    expect(chip.className).not.toContain("bg-emerald-400/20");
    const label = screen.getByTestId(PROMPT_MENTION_LABEL_TEST_ID);
    expect(label.className).toContain("min-w-0");
    expect(label.className).toContain("truncate");
  });

  it("keeps the editable fallback label in a truncation box", () => {
    const name = "missing-prompt-with-a-long-name";
    render(
      <StateProvider initialState={EMPTY_PROMPT_STATE}>
        <PromptMentionChip name={name} value={`@${name}`} presentation="editable" />
      </StateProvider>,
    );

    const chip = screen.getByTestId(PROMPT_MENTION_TEST_ID);
    const label = screen.getByTestId(PROMPT_MENTION_LABEL_TEST_ID);
    expect(chip.getAttribute("title")).toBe(`Custom prompt: ${name}`);
    expect(label.className).toContain("min-w-0");
    expect(label.className).toContain("truncate");
  });
});
