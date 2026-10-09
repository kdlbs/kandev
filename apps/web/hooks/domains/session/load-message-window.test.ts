import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { Message } from "@/lib/types/http";

const listTaskSessionMessages = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessionMessages }));

import { loadMessageWindowAround } from "./load-message-window";
const SESSION = "session";
const TARGET_TIME = "2026-08-22T00:00:00Z";
const MIDDLE_TIME = "2026-08-22T00:00:01Z";
const NEW_TIME = "2026-08-22T00:00:02Z";
const NEWER_UPDATED_TIME = "2026-08-22T00:00:00.123500000Z";
const OLDER_UPDATED_TIME = "2026-08-22T00:00:00.123400000Z";

function message(id: string, created_at: string): Message {
  return { id, created_at, session_id: SESSION } as Message;
}

const mergeMessages = vi.fn();
let existingMessages = [message("new", NEW_TIME)];
const store = {
  getState: () => ({
    messages: { bySession: { [SESSION]: existingMessages } },
    mergeMessages,
  }),
} as unknown as StoreApi<AppState>;

beforeEach(() => {
  existingMessages = [message("new", NEW_TIME)];
  vi.clearAllMocks();
});

describe("loadMessageWindowAround merging", () => {
  it("requests and merges the target-containing around window", async () => {
    listTaskSessionMessages.mockResolvedValue({
      messages: [message("target", TARGET_TIME), message("middle", MIDDLE_TIME)],
    });

    const result = await loadMessageWindowAround(SESSION, "target", () => true, store);

    expect(result).toEqual({ kind: "merged", merged: true, current: true, targetFound: true });
    expect(listTaskSessionMessages).toHaveBeenCalledWith(SESSION, {
      around: "target",
      limit: 100,
      sort: "desc",
    });
    expect(mergeMessages).toHaveBeenCalledWith(SESSION, [
      message("target", TARGET_TIME),
      message("middle", MIDDLE_TIME),
      message("new", NEW_TIME),
    ]);
  });

  it("preserves a newer transcript row when the around response is stale", async () => {
    existingMessages = [
      { ...message("target", TARGET_TIME), content: "new", updated_at: "2026-08-22T00:01:00Z" },
    ];
    listTaskSessionMessages.mockResolvedValue({
      messages: [
        { ...message("target", TARGET_TIME), content: "old", updated_at: "2026-08-22T00:00:30Z" },
      ],
    });

    await loadMessageWindowAround(SESSION, "target", () => true, store);

    expect(mergeMessages).toHaveBeenCalledWith(SESSION, [
      { ...message("target", TARGET_TIME), content: "new", updated_at: "2026-08-22T00:01:00Z" },
    ]);
  });

  it("preserves a newer row when timestamps differ below one millisecond", async () => {
    existingMessages = [
      {
        ...message("target", TARGET_TIME),
        content: "new",
        updated_at: NEWER_UPDATED_TIME,
      },
    ];
    listTaskSessionMessages.mockResolvedValue({
      messages: [
        {
          ...message("target", TARGET_TIME),
          content: "old",
          updated_at: OLDER_UPDATED_TIME,
        },
      ],
    });

    await loadMessageWindowAround(SESSION, "target", () => true, store);

    expect(mergeMessages).toHaveBeenCalledWith(SESSION, [
      {
        ...message("target", TARGET_TIME),
        content: "new",
        updated_at: NEWER_UPDATED_TIME,
      },
    ]);
  });

  it("orders same-millisecond rows by their microsecond creation time", async () => {
    listTaskSessionMessages.mockResolvedValue({
      messages: [message("later", NEWER_UPDATED_TIME), message("earlier", OLDER_UPDATED_TIME)],
    });

    await loadMessageWindowAround(SESSION, "earlier", () => true, store);

    expect(mergeMessages).toHaveBeenCalledWith(SESSION, [
      message("earlier", OLDER_UPDATED_TIME),
      message("later", NEWER_UPDATED_TIME),
      message("new", NEW_TIME),
    ]);
  });
});

describe("loadMessageWindowAround stale boundaries", () => {
  it("does not merge a target returned for a foreign session", async () => {
    listTaskSessionMessages.mockResolvedValue({
      messages: [{ ...message("target", TARGET_TIME), session_id: "foreign" }],
    });

    const result = await loadMessageWindowAround(SESSION, "target", () => true, store);
    expect(result.kind).toBe("deleted-target");
    expect(mergeMessages).not.toHaveBeenCalled();
  });

  it("discards a response whose target changed before it settled", async () => {
    const pending = Promise.withResolvers<{ messages: Message[] }>();
    listTaskSessionMessages.mockReturnValueOnce(pending.promise);
    let current = true;
    const result = loadMessageWindowAround(SESSION, "target", () => current, store);
    current = false;
    pending.resolve({ messages: [message("target", TARGET_TIME)] });
    expect((await result).kind).toBe("stale");
    expect(mergeMessages).not.toHaveBeenCalled();
  });
  it("keeps cached content on equal version while adopting the incoming ordinal", async () => {
    existingMessages = [
      { ...message("target", TARGET_TIME), content: "current", updated_at: NEWER_UPDATED_TIME },
    ];
    listTaskSessionMessages.mockResolvedValue({
      messages: [
        {
          ...message("target", TARGET_TIME),
          content: "stale",
          updated_at: NEWER_UPDATED_TIME,
          prompt_index: 7,
        },
      ],
    });

    await loadMessageWindowAround(SESSION, "target", () => true, store);
    expect(mergeMessages).toHaveBeenCalledWith(SESSION, [
      { ...existingMessages[0], prompt_index: 7 },
    ]);
  });

  it("unions a disjoint around window without synthesizing intermediate rows or changing metadata", async () => {
    existingMessages = [message("new", NEW_TIME)];
    listTaskSessionMessages.mockResolvedValue({ messages: [message("target", TARGET_TIME)] });
    await loadMessageWindowAround(SESSION, "target", () => true, store);

    expect(mergeMessages).toHaveBeenCalledWith(SESSION, [
      message("target", TARGET_TIME),
      message("new", NEW_TIME),
    ]);
    expect(mergeMessages.mock.calls[0]).toHaveLength(2);
  });
});
