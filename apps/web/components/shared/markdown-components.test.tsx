import type { ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { markdownComponents, normalizeMarkdown, remarkPlugins } from "./markdown-components";

vi.mock("@/components/shared/mermaid-block", () => ({
  MermaidBlock: ({ code }: { code: string }) => <div data-kind="mermaid">{code}</div>,
}));

vi.mock("@/components/task/chat/messages/code-block", () => ({
  CodeBlock: ({ children, className }: { children: ReactNode; className?: string }) => (
    <pre data-kind="block" className={className}>
      <code>{children}</code>
    </pre>
  ),
}));

vi.mock("@/components/task/chat/messages/inline-code", () => ({
  InlineCode: ({ children }: { children: ReactNode }) => <code data-kind="inline">{children}</code>,
}));

function renderMarkdown(source: string): string {
  return renderToStaticMarkup(
    <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
      {source}
    </ReactMarkdown>,
  );
}

describe("markdownComponents", () => {
  it("keeps mermaid keywords in inline code as inline code", () => {
    const html = renderMarkdown("Metadata comes from `kanban`, `kanbanMulti`, repositories.");

    expect(html).toContain('data-kind="inline"');
    expect(html).toContain("kanban");
    expect(html).not.toContain('data-kind="mermaid"');
  });

  it("renders fenced mermaid code as a mermaid block", () => {
    const html = renderMarkdown("```mermaid\ngraph LR\nA-->B\n```");

    expect(html).toContain('data-kind="mermaid"');
    expect(html).toContain("graph LR");
  });

  it("renders non-mermaid fenced code as a code block", () => {
    const html = renderMarkdown("```ts\nconst source = 'kanban';\n```");

    expect(html).toContain('data-kind="block"');
    expect(html).toContain("language-ts");
    expect(html).toContain("const source");
    expect(html).not.toContain('data-kind="mermaid"');
  });
});

describe("normalizeMarkdown", () => {
  it("splits a glued 4-backtick close", () => {
    const input = "````go\nfunc f() {\n  ...\n}````\nprose continues here";
    const expected = "````go\nfunc f() {\n  ...\n}\n````\nprose continues here";
    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("splits a glued 3-backtick close", () => {
    const input = "```go\nfunc f() {}```\nprose";
    const expected = "```go\nfunc f() {}\n```\nprose";
    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("leaves a valid fence (close on its own line) unchanged", () => {
    const input = "```go\nfunc f() {}\n```\nprose";
    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("leaves inline-code prose outside any fence unchanged", () => {
    const input = "Use `code` inline in a paragraph.";
    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("does not split when trailing run is shorter than opener", () => {
    // Opens with 4, line ends with 3 — those 3 backticks would not close, so
    // we must not split. The parser will keep gobbling, but at least we don't
    // invent a false close.
    const input = "````go\nfunc f() {}```\nstill code";
    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("splits only the malformed fence when multiple fences appear", () => {
    const input = ["```go", "func f() {}```", "prose", "", "```ts", "const x = 1;", "```"].join(
      "\n",
    );
    const expected = [
      "```go",
      "func f() {}",
      "```",
      "prose",
      "",
      "```ts",
      "const x = 1;",
      "```",
    ].join("\n");
    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("handles up to 3 leading spaces on the opener", () => {
    const input = "   ```go\nfunc f() {}```\nprose";
    const expected = "   ```go\nfunc f() {}\n```\nprose";
    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("returns pure prose unchanged", () => {
    const input = "Just some paragraph text with no fences.\nSecond line.";
    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("returns empty and single-line input unchanged", () => {
    expect(normalizeMarkdown("")).toBe("");
    expect(normalizeMarkdown("single line")).toBe("single line");
  });

  it("preserves a trailing newline if present", () => {
    const input = "```go\nx```\nprose\n";
    const expected = "```go\nx\n```\nprose\n";
    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("renders malformed fence + prose as two code blocks plus a paragraph", () => {
    const malformed = "```go\nfunc f() {}```\nprose continues here\n```ts\nconst x = 1;\n```";
    const htmlRaw = renderMarkdown(malformed);
    const htmlFixed = renderMarkdown(normalizeMarkdown(malformed));

    // Without normalization the parser swallows the prose into one code node.
    const rawBlockCount = (htmlRaw.match(/data-kind="block"/g) ?? []).length;
    const fixedBlockCount = (htmlFixed.match(/data-kind="block"/g) ?? []).length;
    expect(fixedBlockCount).toBeGreaterThan(rawBlockCount);
    expect(fixedBlockCount).toBe(2);

    // Prose paragraph is visible as its own node, not inside a code block.
    expect(htmlFixed).toContain("prose continues here");
    expect(htmlFixed).toContain("language-go");
    expect(htmlFixed).toContain("language-ts");
  });
});
