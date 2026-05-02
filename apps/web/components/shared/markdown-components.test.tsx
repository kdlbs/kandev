import ReactMarkdown from "react-markdown";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { markdownComponents, remarkPlugins } from "./markdown-components";

vi.mock("@/components/shared/mermaid-block", () => ({
  MermaidBlock: ({ code }: { code: string }) => <div data-kind="mermaid">{code}</div>,
}));

vi.mock("@/components/task/chat/messages/code-block", () => ({
  CodeBlock: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <pre data-kind="block" className={className}>
      <code>{children}</code>
    </pre>
  ),
}));

vi.mock("@/components/task/chat/messages/inline-code", () => ({
  InlineCode: ({ children }: { children: React.ReactNode }) => (
    <code data-kind="inline">{children}</code>
  ),
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
});
