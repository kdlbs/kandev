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
  return { id, created_at } as Message;
}

describe("loadMessageWindowAround", () => {
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
  });

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
