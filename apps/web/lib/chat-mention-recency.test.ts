import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MentionItem } from "@/hooks/use-inline-mention";
import {
  CHAT_MENTION_RECENCY_STORAGE_KEY,
  MAX_CHAT_MENTION_RECENT_ENTRIES,
  getChatMentionRecentEntries,
  mentionItemToRecentEntry,
  normalizeChatMentionRecentEntries,
  rankMentionItems,
  recordChatMentionSelection,
} from "./chat-mention-recency";

function makeItem(
  kind: MentionItem["kind"],
  id: string,
  label: string,
  taskId?: string,
): MentionItem {
  return {
    id,
    kind,
    label,
    task:
      kind === "task"
        ? {
            taskId: taskId ?? id,
            title: label,
            workflowId: "workflow-1",
            workflowStepId: "step-1",
            state: null,
          }
        : undefined,
    onSelect: vi.fn(),
  };
}

const WORKSPACE_ID = "workspace-1";
const OTHER_WORKSPACE_ID = "workspace-2";
const FILE_PATH = "src/app.ts";

describe("chat mention recency", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("preserves baseline filtering and stable source order without history", () => {
    const items = [
      makeItem("prompt", "contains", "Acontainsneedle"),
      makeItem("prompt", "word", "A needle word"),
      makeItem("prompt", "prefix", "Needle first"),
      makeItem("prompt", "same-1", "Same needle"),
      makeItem("prompt", "same-2", "Another needle"),
    ];

    expect(rankMentionItems(items, "needle", WORKSPACE_ID, [])).toEqual([
      items[2],
      items[1],
      items[3],
      items[4],
      items[0],
    ]);
  });

  it("ranks a recent weaker text match before an unselected stronger match", () => {
    const recent = makeItem("task", "task:recent", "Project task", "recent");
    const stronger = makeItem("task", "task:stronger", "Task owner", "stronger");

    expect(
      rankMentionItems([stronger, recent], "task", WORKSPACE_ID, [
        {
          kind: "task",
          id: "recent",
        },
      ]),
    ).toEqual([recent, stronger]);
  });

  it("orders multiple recent candidates by newest selection before text score", () => {
    const older = makeItem("task", "task:older", "Older task", "older");
    const newer = makeItem("prompt", "prompt:newer", "Newer prompt");
    const unselected = makeItem("task", "task:unselected", "Unselected task", "unselected");

    expect(
      rankMentionItems([older, newer, unselected], "", WORKSPACE_ID, [
        { kind: "prompt", id: newer.id },
        { kind: "task", id: "older" },
      ]),
    ).toEqual([newer, older, unselected]);
  });

  it("keeps the Plan action at its baseline index while reordering candidates", () => {
    const first = makeItem("task", "task:first", "First task", "first");
    const plan = makeItem("plan", "__plan__", "Plan");
    const recent = makeItem("prompt", "prompt-recent", "Recent prompt");

    expect(
      rankMentionItems([first, plan, recent], "", WORKSPACE_ID, [
        { kind: "prompt", id: recent.id },
      ]),
    ).toEqual([recent, plan, first]);
  });

  it("uses Plan's filtered baseline position for non-empty queries", () => {
    const filtered = makeItem("task", "task:filtered", "Unrelated task", "filtered");
    const plan = makeItem("plan", "__plan__", "Plan");
    const recent = makeItem("prompt", "prompt-recent", "Plan prompt");

    expect(
      rankMentionItems([filtered, plan, recent], "plan", WORKSPACE_ID, [
        { kind: "prompt", id: recent.id },
      ]),
    ).toEqual([plan, recent]);
  });

  it("scopes file identities to the active workspace", () => {
    const sharedPath = makeItem("file", FILE_PATH, FILE_PATH);
    const otherPath = makeItem("file", "src/other.ts", "src/other.ts");

    expect(
      rankMentionItems([otherPath, sharedPath], "", OTHER_WORKSPACE_ID, [
        { kind: "file", id: FILE_PATH, workspaceId: WORKSPACE_ID },
      ]),
    ).toEqual([otherPath, sharedPath]);
    expect(
      rankMentionItems([otherPath, sharedPath], "", WORKSPACE_ID, [
        { kind: "file", id: FILE_PATH, workspaceId: WORKSPACE_ID },
      ]),
    ).toEqual([sharedPath, otherPath]);
  });
});

describe("chat mention recency storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("normalizes malformed history, removes duplicate identities, and bounds the list", () => {
    const entries = normalizeChatMentionRecentEntries([
      { kind: "prompt", id: "prompt-1" },
      { kind: "prompt", id: "prompt-1" },
      { kind: "file", id: FILE_PATH },
      { kind: "file", id: FILE_PATH, workspaceId: WORKSPACE_ID },
      { kind: "file", id: FILE_PATH, workspaceId: OTHER_WORKSPACE_ID },
      { kind: "unknown", id: "bad" },
      { kind: "task", id: "" },
      null,
    ]);

    expect(entries).toEqual([
      { kind: "prompt", id: "prompt-1" },
      { kind: "file", id: FILE_PATH, workspaceId: WORKSPACE_ID },
      { kind: "file", id: FILE_PATH, workspaceId: OTHER_WORKSPACE_ID },
    ]);

    const bounded = normalizeChatMentionRecentEntries(
      Array.from({ length: MAX_CHAT_MENTION_RECENT_ENTRIES + 3 }, (_, index) => ({
        kind: "task",
        id: `task-${index}`,
      })),
    );
    expect(bounded).toHaveLength(MAX_CHAT_MENTION_RECENT_ENTRIES);
    expect(bounded[0]).toEqual({ kind: "task", id: "task-0" });
  });

  it("persists selections as a bounded newest-first MRU list", () => {
    const first = makeItem("task", "task:first", "First", "first");
    const second = makeItem("prompt", "prompt:second", "Second");
    window.localStorage.setItem(
      CHAT_MENTION_RECENCY_STORAGE_KEY,
      JSON.stringify(
        Array.from({ length: MAX_CHAT_MENTION_RECENT_ENTRIES }, (_, index) => ({
          kind: "task",
          id: `task-${index}`,
        })),
      ),
    );

    recordChatMentionSelection(first, WORKSPACE_ID);
    recordChatMentionSelection(second, WORKSPACE_ID);
    recordChatMentionSelection(first, WORKSPACE_ID);

    const stored = getChatMentionRecentEntries();
    expect(stored[0]).toEqual({ kind: "task", id: "first" });
    expect(stored[1]).toEqual({ kind: "prompt", id: "prompt:second" });
    expect(stored).toHaveLength(MAX_CHAT_MENTION_RECENT_ENTRIES);
    expect(stored.filter((entry) => entry.kind === "task" && entry.id === "first")).toHaveLength(1);
  });

  it("does not record Plan or an unscoped file", () => {
    const plan = makeItem("plan", "__plan__", "Plan");
    const file = makeItem("file", FILE_PATH, FILE_PATH);

    expect(mentionItemToRecentEntry(plan, WORKSPACE_ID)).toBeNull();
    expect(mentionItemToRecentEntry(file, null)).toBeNull();
    recordChatMentionSelection(plan, WORKSPACE_ID);
    recordChatMentionSelection(file, null);
    expect(getChatMentionRecentEntries()).toEqual([]);
  });

  it("continues without throwing when browser storage is unavailable", () => {
    const originalStorage = window.localStorage;
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      get: () => {
        throw new Error("storage unavailable");
      },
    });

    try {
      expect(getChatMentionRecentEntries()).toEqual([]);
      expect(() =>
        recordChatMentionSelection(makeItem("prompt", "prompt-1", "Prompt"), "w-1"),
      ).not.toThrow();
    } finally {
      Object.defineProperty(window, "localStorage", {
        configurable: true,
        value: originalStorage,
      });
    }
  });
});
