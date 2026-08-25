import { describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import { doFetchMessages } from "./use-session-message-fetch";

const SESSION_ID = "session-1";

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function makeParams(
  fetchAndStoreMessages: (
    sessionId: string,
    store: never,
    isActive?: () => boolean,
  ) => Promise<Message[]>,
  setMessagesLoading: ReturnType<typeof vi.fn>,
) {
  return {
    taskSessionId: SESSION_ID,
    store: {
      getState: () => ({ setMessagesLoading, setMessages: vi.fn() }),
    } as never,
    setIsLoading: vi.fn(),
    setIsWaitingForInitialMessages: vi.fn(),
    initialFetchStartRef: { current: null },
    lastFetchedSessionIdRef: { current: null },
    fetchAndStoreMessages,
  };
}

describe("doFetchMessages", () => {
  it("settles a tool-only initial fetch without waiting for older history", async () => {
    const setMessagesLoading = vi.fn();
    const params = makeParams(
      vi.fn().mockResolvedValue([
        {
          id: "tool-1",
          type: "tool_call",
          author_type: "agent",
        } as Message,
      ]),
      setMessagesLoading,
    );

    await doFetchMessages(params as never);

    expect(setMessagesLoading).toHaveBeenLastCalledWith(SESSION_ID, false);
  });

  it("keeps the shared loading flag set until overlapping fetches all settle", async () => {
    const first = deferred<Message[]>();
    const second = deferred<Message[]>();
    const fetchAndStoreMessages = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    const setMessagesLoading = vi.fn();

    const firstFetch = doFetchMessages(
      makeParams(fetchAndStoreMessages, setMessagesLoading) as never,
    );
    const secondFetch = doFetchMessages(
      makeParams(fetchAndStoreMessages, setMessagesLoading) as never,
    );

    expect(setMessagesLoading).toHaveBeenNthCalledWith(1, SESSION_ID, true);
    expect(setMessagesLoading).toHaveBeenNthCalledWith(2, SESSION_ID, true);

    first.resolve([]);
    await firstFetch;
    expect(setMessagesLoading).toHaveBeenCalledTimes(2);

    second.resolve([]);
    await secondFetch;
    expect(setMessagesLoading).toHaveBeenLastCalledWith(SESSION_ID, false);
  });
});
