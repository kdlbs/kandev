import { describe, expect, it } from "vitest";
import {
  createMessageHistorySelector,
  extractUserHistory,
  navigateHistory,
  fuzzyScore,
  searchHistory,
  type HistoryState,
} from "./message-history";
import * as messageHistory from "./message-history";
import type { Message } from "@/lib/types/http";

function msg(partial: Partial<Message>): Message {
  return {
    id: partial.id ?? "m",
    session_id: "s1" as Message["session_id"],
    task_id: "t1" as Message["task_id"],
    author_type: partial.author_type ?? "user",
    type: partial.type ?? "message",
    content: partial.content ?? "",
    created_at: partial.created_at ?? "2026-01-01T00:00:00Z",
    ...partial,
  };
}

describe("createMessageHistorySelector", () => {
  it("keeps the selector snapshot stable when only agent output changes", () => {
    const selector = createMessageHistorySelector("s1");
    const first = selector({
      messages: {
        bySession: { s1: [msg({ id: "user-1", content: "keep me" })] },
        metaBySession: {},
      },
    });
    const afterAgentStream = selector({
      messages: {
        bySession: {
          s1: [
            msg({ id: "user-1", content: "keep me" }),
            msg({ id: "agent-1", author_type: "agent", content: "streaming" }),
          ],
        },
        metaBySession: {},
      },
    });

    expect(afterAgentStream).toBe(first);
  });

  it("returns a new selector snapshot when reference metadata changes", () => {
    const selector = createMessageHistorySelector("s1");
    const reference = {
      version: 1,
      ref: "mention:v1:sentry:issue:sentry.example.test%2Forg-1:issue-1",
      provider: "sentry",
      kind: "issue",
      id: "issue-1",
      key: "WEB-1",
      title: "Login crash",
      url: "https://sentry.example.test/issues/1",
      scope: "sentry.example.test/org-1",
    };
    const first = selector({
      messages: {
        bySession: {
          s1: [msg({ id: "user-1", content: "See #WEB-1", metadata: { entity_references: [] } })],
        },
        metaBySession: {},
      },
    });
    const withReference = selector({
      messages: {
        bySession: {
          s1: [
            msg({
              id: "user-1",
              content: "See #WEB-1",
              metadata: { entity_references: [reference] },
            }),
          ],
        },
        metaBySession: {},
      },
    });

    expect(withReference).not.toBe(first);
    expect(withReference[0]?.entityReferences).toEqual([reference]);
  });
});

describe("extractUserHistoryEntries", () => {
  it("provides structured history entries for rich-editor recall", () => {
    expect(typeof (messageHistory as Record<string, unknown>).extractUserHistoryEntries).toBe(
      "function",
    );
  });

  it("keeps entity reference metadata with recalled message content", () => {
    const reference = {
      version: 1,
      ref: "mention:v1:sentry:issue:sentry.example.test%2Forg-1:issue-1",
      provider: "sentry",
      kind: "issue",
      id: "issue-1",
      key: "WEB-1",
      title: "Login crash",
      url: "https://sentry.example.test/issues/1",
      scope: "sentry.example.test/org-1",
    };
    const entries = messageHistory.extractUserHistoryEntries([
      msg({
        content: "See [#WEB-1](https://sentry.example.test/issues/1)",
        metadata: { entity_references: [reference, reference] },
      }),
    ]);

    expect(entries).toEqual([
      {
        content: "See [#WEB-1](https://sentry.example.test/issues/1)",
        entityReferences: [reference],
      },
    ]);
  });
});

describe("extractUserHistory", () => {
  it("returns user message contents newest-first", () => {
    const messages: Message[] = [
      msg({ id: "1", content: "first" }),
      msg({ id: "2", content: "second" }),
      msg({ id: "3", content: "third" }),
    ];
    expect(extractUserHistory(messages)).toEqual(["third", "second", "first"]);
  });

  it("skips agent messages", () => {
    const messages: Message[] = [
      msg({ id: "1", content: "user-a" }),
      msg({ id: "2", content: "agent-reply", author_type: "agent" }),
      msg({ id: "3", content: "user-b" }),
    ];
    expect(extractUserHistory(messages)).toEqual(["user-b", "user-a"]);
  });

  it("skips non-message types (tool_call, thinking, etc.)", () => {
    const messages: Message[] = [
      msg({ id: "1", content: "real prompt" }),
      msg({ id: "2", content: "thought", type: "thinking" }),
      msg({ id: "3", content: "ran cmd", type: "tool_execute" }),
    ];
    expect(extractUserHistory(messages)).toEqual(["real prompt"]);
  });

  it("skips empty/whitespace content", () => {
    const messages: Message[] = [
      msg({ id: "1", content: "real" }),
      msg({ id: "2", content: "" }),
      msg({ id: "3", content: "   " }),
    ];
    expect(extractUserHistory(messages)).toEqual(["real"]);
  });

  it("collapses consecutive duplicates", () => {
    const messages: Message[] = [
      msg({ id: "1", content: "fix bug" }),
      msg({ id: "2", content: "fix bug" }),
      msg({ id: "3", content: "fix bug" }),
      msg({ id: "4", content: "ship it" }),
    ];
    expect(extractUserHistory(messages)).toEqual(["ship it", "fix bug"]);
  });

  it("keeps non-consecutive duplicates", () => {
    const messages: Message[] = [
      msg({ id: "1", content: "a" }),
      msg({ id: "2", content: "b" }),
      msg({ id: "3", content: "a" }),
    ];
    expect(extractUserHistory(messages)).toEqual(["a", "b", "a"]);
  });

  it("keeps duplicates separated by agent/tool turns (adjacency is in original stream)", () => {
    const messages: Message[] = [
      msg({ id: "1", content: "fix bug" }),
      msg({ id: "2", content: "working on it", author_type: "agent" }),
      msg({ id: "3", content: "ran cmd", type: "tool_execute" }),
      msg({ id: "4", content: "fix bug" }),
    ];
    expect(extractUserHistory(messages)).toEqual(["fix bug", "fix bug"]);
  });

  it("returns empty for empty input", () => {
    expect(extractUserHistory([])).toEqual([]);
  });
});

describe("navigateHistory", () => {
  const empty: HistoryState = { index: null };

  it("returns null when there is no history", () => {
    expect(navigateHistory(empty, "up", 0)).toBeNull();
    expect(navigateHistory(empty, "down", 0)).toBeNull();
  });

  it("first ArrowUp enters history at index 0", () => {
    expect(navigateHistory(empty, "up", 3)).toEqual({ index: 0 });
  });

  it("subsequent ArrowUp moves further back", () => {
    expect(navigateHistory({ index: 0 }, "up", 3)).toEqual({ index: 1 });
    expect(navigateHistory({ index: 1 }, "up", 3)).toEqual({ index: 2 });
  });

  it("ArrowUp at the oldest entry returns null", () => {
    expect(navigateHistory({ index: 2 }, "up", 3)).toBeNull();
  });

  it("ArrowDown from null defers (returns null)", () => {
    expect(navigateHistory(empty, "down", 3)).toBeNull();
  });

  it("ArrowDown moves toward newer entries", () => {
    expect(navigateHistory({ index: 2 }, "down", 3)).toEqual({ index: 1 });
    expect(navigateHistory({ index: 1 }, "down", 3)).toEqual({ index: 0 });
  });

  it("ArrowDown from index 0 exits history (restores draft)", () => {
    expect(navigateHistory({ index: 0 }, "down", 3)).toEqual({ index: null });
  });
});

describe("fuzzyScore", () => {
  it("returns a positive score for an empty needle", () => {
    expect(fuzzyScore("", "anything")).toBeGreaterThan(0);
  });

  it("matches subsequences in order, case-insensitive", () => {
    expect(fuzzyScore("fb", "fix bug")).not.toBeNull();
    expect(fuzzyScore("FB", "Fix Bug")).not.toBeNull();
  });

  it("returns null when characters cannot be matched in order", () => {
    expect(fuzzyScore("xyz", "fix bug")).toBeNull();
    expect(fuzzyScore("gf", "fix bug")).toBeNull(); // g comes after f in haystack? no - 'g' is in 'bug', 'f' is before
  });

  it("ranks word-boundary matches above mid-word ones", () => {
    const boundary = fuzzyScore("fb", "fix bug")!;
    const midword = fuzzyScore("fb", "affable")!;
    expect(boundary).not.toBeNull();
    expect(midword).not.toBeNull();
    expect(boundary).toBeGreaterThan(midword);
  });

  it("ranks consecutive matches above scattered ones", () => {
    const consecutive = fuzzyScore("fix", "fix bug")!;
    const scattered = fuzzyScore("fix", "f i x in mix")!;
    expect(consecutive).toBeGreaterThan(scattered);
  });
});

describe("searchHistory", () => {
  it("returns every entry unchanged for an empty query", () => {
    const history = ["c", "b", "a"];
    expect(searchHistory(history, "")).toEqual([
      { index: 0, content: "c", score: 0 },
      { index: 1, content: "b", score: 0 },
      { index: 2, content: "a", score: 0 },
    ]);
  });

  it("filters out non-matching entries", () => {
    const history = ["fix bug", "ship it", "rename foo"];
    const hits = searchHistory(history, "fb");
    expect(hits.map((h) => h.content)).toEqual(["fix bug"]);
  });

  it("orders matches best-first (consecutive chars beat scattered)", () => {
    const history = ["find the bug later", "fb shortcut win"];
    const hits = searchHistory(history, "fb");
    expect(hits[0].content).toBe("fb shortcut win");
  });

  it("preserves the original index so callers can jump history position", () => {
    const history = ["a", "fb match", "c"];
    const [hit] = searchHistory(history, "fb");
    expect(hit.index).toBe(1);
  });
});
