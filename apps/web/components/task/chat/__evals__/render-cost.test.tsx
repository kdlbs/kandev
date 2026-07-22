/**
 * Render-cost eval harness — the objective guard against the markdown
 * re-render/re-parse "storm".
 *
 * It mounts a list of memoized markdown rows and asserts how many real
 * normalize passes (cache misses) and per-row re-renders happen under each
 * update pattern. Wave 1 owns the parse-count assertions; Wave 2/3 extend the
 * row-render assertions once callbacks + store identity are stabilized.
 */
import { createContext, memo, useState } from "react";
import { act, fireEvent, render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("react-markdown", () => ({
  default: ({ children }: { children: string }) => <div>{children}</div>,
}));

vi.mock("@/components/shared/markdown-components", () => ({
  MarkdownFileLinkContext: createContext({}),
  markdownComponents: {},
  remarkPlugins: [],
}));

import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { __markdownParseCount, __resetMarkdownCounters } from "@/lib/markdown/normalize-cache";
import {
  createMessageTextAnchor,
  getMessageCommentDecorations,
} from "@/lib/chat/agent-message-comments";

type Row = { id: string; content: string };

const rowRenderCounts = new Map<string, number>();

const MarkdownRow = memo(function MarkdownRow({ id, content }: Row) {
  rowRenderCounts.set(id, (rowRenderCounts.get(id) ?? 0) + 1);
  return <MemoizedMarkdown content={content} />;
});

function MessageList({ rows, panel }: { rows: Row[]; panel: boolean }) {
  return (
    <div data-panel={String(panel)}>
      {rows.map((r) => (
        <MarkdownRow key={r.id} id={r.id} content={r.content} />
      ))}
    </div>
  );
}

function makeRows(n: number): Row[] {
  return Array.from({ length: n }, (_, i) => ({ id: `m${i}`, content: `# Message ${i}` }));
}

/** Deep clone into fresh object refs but identical content (simulates a refetch). */
function refetch(rows: Row[]): Row[] {
  return rows.map((r) => ({ id: r.id, content: r.content }));
}

function totalRowRenders(): number {
  let total = 0;
  for (const count of rowRenderCounts.values()) total += count;
  return total;
}

describe("chat render-cost evals", () => {
  beforeEach(() => {
    __resetMarkdownCounters();
    rowRenderCounts.clear();
  });

  it("scenario 1: no-op refetch causes 0 parses and 0 row re-renders", () => {
    const rows = makeRows(20);
    const { rerender } = render(<MessageList rows={rows} panel={false} />);
    const parsesAfterMount = __markdownParseCount();
    rowRenderCounts.clear();

    rerender(<MessageList rows={refetch(rows)} panel={false} />);

    expect(__markdownParseCount() - parsesAfterMount).toBe(0);
    expect(totalRowRenders()).toBe(0);
  });

  it("scenario 2: one changed message causes exactly 1 new parse", () => {
    const rows = makeRows(20);
    const { rerender } = render(<MessageList rows={rows} panel={false} />);
    const parsesAfterMount = __markdownParseCount();
    rowRenderCounts.clear();

    const next = refetch(rows);
    next[5] = { id: next[5].id, content: "# Message 5 (edited)" };
    rerender(<MessageList rows={next} panel={false} />);

    expect(__markdownParseCount() - parsesAfterMount).toBe(1);
    expect(rowRenderCounts.get("m5")).toBe(1);
  });

  it("scenario 3: unrelated parent re-render causes 0 parses", () => {
    const rows = makeRows(20);
    function Harness() {
      const [panel, setPanel] = useState(false);
      return (
        <div>
          <button onClick={() => setPanel((p) => !p)}>toggle</button>
          <MessageList rows={rows} panel={panel} />
        </div>
      );
    }
    const { getByText } = render(<Harness />);
    const parsesAfterMount = __markdownParseCount();
    rowRenderCounts.clear();

    act(() => {
      fireEvent.click(getByText("toggle"));
    });

    expect(__markdownParseCount() - parsesAfterMount).toBe(0);
    expect(totalRowRenders()).toBe(0);
  });

  it("scenario 4: identical content across two rows parses once (cache hit)", () => {
    const rows: Row[] = [
      { id: "a", content: "# Same content" },
      { id: "b", content: "# Same content" },
    ];
    render(<MessageList rows={rows} panel={false} />);
    expect(__markdownParseCount()).toBe(1);
  });

  it("scenario 5: restoring many inline highlights does not reparse markdown rows", () => {
    const content = Array.from({ length: 20 }, (_, index) => `reply-${index}`).join(" ");
    const root = document.createElement("div");
    root.textContent = content;
    const comments = Array.from({ length: 20 }, (_, index) => {
      const selectedText = `reply-${index}`;
      const start = content.indexOf(selectedText);
      return {
        id: `comment-${index}`,
        sessionId: "session-1",
        source: "agent-message" as const,
        messageId: "reply-1",
        selectedText,
        text: "feedback",
        createdAt: "2026-07-20T00:00:00Z",
        status: "pending" as const,
        anchor: createMessageTextAnchor("reply-1", content, start, start + selectedText.length),
      };
    });

    const parsesBefore = __markdownParseCount();
    expect(getMessageCommentDecorations(root, comments)).toHaveLength(20);
    expect(root.querySelectorAll("mark[data-agent-message-comment-id]")).toHaveLength(0);
    expect(__markdownParseCount() - parsesBefore).toBe(0);
  });
});
