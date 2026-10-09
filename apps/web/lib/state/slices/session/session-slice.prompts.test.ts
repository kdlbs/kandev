import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";
import { sessionId, taskId, type Message } from "@/lib/types/http";

const SESSION = "prompts-session";
const SECOND_PROMPT_TIME = "2026-08-22T00:00:01Z";

function makeStore() {
  return create<SessionSlice>()(
    // Zustand's immer middleware erases the slice mutator tuple in this test harness.
    immer((set) => ({
      ...(createSessionSlice as unknown as (setter: typeof set) => SessionSlice)(set),
    })),
  );
}

function message(id: string, author_type: "user" | "agent"): Message {
  return {
    id,
    session_id: sessionId(SESSION),
    task_id: taskId("task"),
    author_type,
    type: "message",
    content: id,
    created_at: "2026-08-22T00:00:00Z",
  } as Message;
}

describe("prompt cache fan-out", () => {
  it("keeps only user messages in the independent prompts cache", () => {
    const store = makeStore();
    store.getState().addMessage(message("user", "user"));
    store.getState().addMessage(message("agent", "agent"));

    expect(store.getState().messagePrompts.bySession[SESSION].map((entry) => entry.id)).toEqual([
      "user",
    ]);
    expect(store.getState().messages.bySession[SESSION]).toHaveLength(2);
  });
});

it("records a live prompt as observed without marking the projection authoritative", () => {
  const store = makeStore();
  store.getState().addMessage(message("live", "user"));

  expect(store.getState().messagePrompts.observedBySession?.[SESSION]?.ids).toEqual({
    live: true,
  });
  expect(store.getState().messagePrompts.authoritativeBySession?.[SESSION]).toBeUndefined();
});

it("updates prompts even when the transcript cache is absent", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(SESSION, [message("user", "user")]);
  store.setState((state) => ({ ...state, messages: { bySession: {}, metaBySession: {} } }));

  store.getState().updateMessage({ ...message("user", "user"), content: "updated" });

  expect(store.getState().messagePrompts.bySession[SESSION][0].content).toBe("updated");
});

it("unions user rows from transcript snapshots without dropping deep prompt pages", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(SESSION, [message("deep", "user")]);

  store
    .getState()
    .mergeMessages(SESSION, [
      { ...message("new", "user"), created_at: SECOND_PROMPT_TIME },
      message("agent", "agent"),
    ]);

  expect(store.getState().messagePrompts.bySession[SESSION].map((entry) => entry.id)).toEqual([
    "deep",
    "new",
  ]);
});

it("installs an authoritative page without resetting an older-page cursor", () => {
  const store = makeStore();
  store
    .getState()
    .replacePromptMessages(
      SESSION,
      [{ ...message("current", "user"), created_at: SECOND_PROMPT_TIME }],
      { hasMore: true, oldestCursor: "current" },
    );
  store.getState().prependPromptMessages(SESSION, [message("older", "user")], {
    hasMore: true,
    oldestCursor: "older",
  });
  store
    .getState()
    .installAuthoritativePromptMessages(
      SESSION,
      [{ ...message("new", "user"), created_at: "2026-08-22T00:00:02Z" }],
      { hasMore: false, oldestCursor: "new" },
    );

  expect(store.getState().messagePrompts.bySession[SESSION].map((row) => row.id)).toEqual([
    "older",
    "current",
    "new",
  ]);
  expect(store.getState().messagePrompts.metaBySession[SESSION].oldestCursor).toBe("older");
  expect(store.getState().messagePrompts.authoritativeBySession[SESSION]).toBe(true);
  expect(store.getState().messagePrompts.observedBySession[SESSION].ids).toEqual({ new: true });
});

it("keeps the newest observed floor across a delayed older authority page", () => {
  const store = makeStore();
  store.getState().addMessage({ ...message("live", "user"), created_at: "2026-08-22T00:00:03Z" });
  store
    .getState()
    .installAuthoritativePromptMessages(
      SESSION,
      [{ ...message("old", "user"), created_at: SECOND_PROMPT_TIME }],
      { hasMore: false, oldestCursor: null },
    );

  expect(store.getState().messagePrompts.observedBySession[SESSION].newestKey).toEqual({
    id: "live",
    created_at: "2026-08-22T00:00:03Z",
  });
});

it("does not admit foreign projection rows or promote fan-out cache entries on an empty read", () => {
  const store = makeStore();
  store.getState().mergeMessages(SESSION, [message("fanout", "user")]);
  store.getState().installAuthoritativePromptMessages(
    SESSION,
    [
      {
        ...message("foreign", "user"),
        session_id: "other" as Message["session_id"],
        created_at: SECOND_PROMPT_TIME,
      },
    ],
    { hasMore: false, oldestCursor: null },
  );

  expect(store.getState().messagePrompts.bySession[SESSION].map((row) => row.id)).toEqual([
    "fanout",
  ]);
  expect(store.getState().messagePrompts.observedBySession[SESSION]).toEqual({
    ids: {},
    newestKey: null,
  });
});

it("keeps older history available without reviving a deleted cursor from a held read", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(SESSION, [message("removed", "user")], {
    hasMore: true,
    oldestCursor: "removed",
  });
  store.getState().removeMessage(SESSION, "removed");
  store.getState().installAuthoritativePromptMessages(SESSION, [message("removed", "user")], {
    hasMore: true,
    oldestCursor: "removed",
  });

  expect(store.getState().messagePrompts.bySession[SESSION]).toEqual([]);
  expect(store.getState().messagePrompts.metaBySession[SESSION]).toMatchObject({
    historyInitialized: true,
    hasMore: true,
    oldestCursor: null,
  });
});

it("repairs the prompt cursor after deleting the oldest cached prompt", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(
    SESSION,
    [
      { ...message("oldest", "user"), created_at: "2026-08-22T00:00:00Z" },
      { ...message("newest", "user"), created_at: SECOND_PROMPT_TIME },
    ],
    { hasMore: true, oldestCursor: "oldest" },
  );

  store.getState().removeMessage(SESSION, "oldest");

  expect(store.getState().messagePrompts.metaBySession[SESSION].oldestCursor).toBe("newest");
});

it("keeps older prompt history available after deleting its only cached cursor", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(SESSION, [message("newest", "user")], {
    hasMore: true,
    oldestCursor: "newest",
  });

  store.getState().removeMessage(SESSION, "newest");

  expect(store.getState().messagePrompts.bySession[SESSION]).toEqual([]);
  expect(store.getState().messagePrompts.metaBySession[SESSION]).toMatchObject({
    hasMore: true,
    oldestCursor: null,
  });
});

it("installs authoritative pagination metadata over a live prompt cache", () => {
  const store = makeStore();
  store.getState().addMessage(message("live", "user"));

  store
    .getState()
    .installAuthoritativePromptMessages(
      SESSION,
      [{ ...message("newest", "user"), created_at: SECOND_PROMPT_TIME }],
      { hasMore: true, oldestCursor: "newest" },
    );

  expect(store.getState().messagePrompts.metaBySession[SESSION]).toMatchObject({
    historyInitialized: true,
    hasMore: true,
    oldestCursor: "newest",
  });
});

it("does not install a deleted live prompt as the pagination cursor", () => {
  const store = makeStore();
  const deleted = message("deleted-live", "user");
  store.getState().addMessage(deleted);
  store.getState().removeMessage(SESSION, deleted.id);
  store.getState().installAuthoritativePromptMessages(SESSION, [deleted], {
    hasMore: true,
    oldestCursor: deleted.id,
  });

  expect(store.getState().messagePrompts.bySession[SESSION]).toEqual([]);
  expect(store.getState().messagePrompts.metaBySession[SESSION]).toMatchObject({
    historyInitialized: true,
    hasMore: true,
    oldestCursor: null,
  });
});

it("replaces the authoritative prompt window instead of retaining deleted rows", () => {
  const store = makeStore();
  store
    .getState()
    .replacePromptMessages(SESSION, [
      message("deleted", "user"),
      { ...message("kept", "user"), created_at: SECOND_PROMPT_TIME },
    ]);

  store
    .getState()
    .replacePromptMessages(SESSION, [
      { ...message("kept", "user"), created_at: SECOND_PROMPT_TIME },
    ]);

  expect(store.getState().messagePrompts.bySession[SESSION].map((entry) => entry.id)).toEqual([
    "kept",
  ]);
});

it("preserves a newer cached row when a replacement response is stale", () => {
  const store = makeStore();
  store.getState().addMessage({
    ...message("prompt", "user"),
    content: "new",
    updated_at: "2026-08-22T00:01:00Z",
  });

  store.getState().replacePromptMessages(SESSION, [
    {
      ...message("prompt", "user"),
      content: "old",
      updated_at: "2026-08-22T00:00:30Z",
    },
  ]);

  expect(store.getState().messagePrompts.bySession[SESSION][0].content).toBe("new");
});

it("does not regress a prompt when an older update arrives", () => {
  const store = makeStore();
  store.getState().addMessage({
    ...message("prompt", "user"),
    content: "new",
    updated_at: "2026-08-22T00:01:00Z",
  });

  store.getState().updateMessage({
    ...message("prompt", "user"),
    content: "old",
    updated_at: "2026-08-22T00:00:30Z",
  });

  expect(store.getState().messagePrompts.bySession[SESSION][0].content).toBe("new");
});
it("keeps a cached prompt on an equal-version authority response", () => {
  const store = makeStore();
  store.getState().addMessage({
    ...message("prompt", "user"),
    content: "current",
    updated_at: SECOND_PROMPT_TIME,
  });

  store
    .getState()
    .replacePromptMessages(SESSION, [
      { ...message("prompt", "user"), content: "stale", updated_at: SECOND_PROMPT_TIME },
    ]);

  expect(store.getState().messagePrompts.bySession[SESSION][0].content).toBe("current");
});

it("does not resurrect a deleted prompt from a held projection response", () => {
  const store = makeStore();
  store.getState().removeMessage(SESSION, "deleted");
  store.getState().replacePromptMessages(SESSION, [message("deleted", "user")]);

  expect(store.getState().messagePrompts.bySession[SESSION]).toEqual([]);
});

it("uses exhausted page metadata after an empty older page", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(SESSION, [message("oldest", "user")], {
    hasMore: true,
    oldestCursor: "oldest",
  });
  store.getState().prependPromptMessages(SESSION, [], { hasMore: false, oldestCursor: null });

  expect(store.getState().messagePrompts.metaBySession[SESSION]).toMatchObject({
    hasMore: false,
    oldestCursor: null,
  });
});

it("does not regress a prompt when an update is older by less than one millisecond", () => {
  const store = makeStore();
  store.getState().addMessage({
    ...message("prompt", "user"),
    content: "new",
    updated_at: "2026-08-22T00:00:00.123500000Z",
  });

  store.getState().updateMessage({
    ...message("prompt", "user"),
    content: "old",
    updated_at: "2026-08-22T00:00:00.123400000Z",
  });

  expect(store.getState().messagePrompts.bySession[SESSION][0].content).toBe("new");
});

it("orders prompts by microsecond creation time before using the id tie-break", () => {
  const store = makeStore();
  store.getState().addMessage({
    ...message("later", "user"),
    created_at: "2026-08-22T00:00:00.123500000Z",
  });
  store.getState().addMessage({
    ...message("earlier", "user"),
    created_at: "2026-08-22T00:00:00.123400000Z",
  });

  expect(store.getState().messagePrompts.bySession[SESSION].map((entry) => entry.id)).toEqual([
    "earlier",
    "later",
  ]);
});

it("advances the refresh revision for live prompt mutations", () => {
  const store = makeStore();
  const initialRevision = store.getState().messagePrompts.refreshGenerationBySession[SESSION] ?? 0;

  store.getState().addMessage(message("live", "user"));
  expect(store.getState().messagePrompts.refreshGenerationBySession[SESSION]).toBe(
    initialRevision + 1,
  );

  store.getState().removeMessage(SESSION, "live");
  expect(store.getState().messagePrompts.refreshGenerationBySession[SESSION]).toBe(
    initialRevision + 2,
  );
});

it("advances the refresh revision when an initial prompt load starts", () => {
  const store = makeStore();
  store.getState().setPromptMessagesLoading(SESSION, true);
  const revision = store.getState().messagePrompts.refreshGenerationBySession[SESSION];

  store.getState().setPromptMessagesLoading(SESSION, true);
  expect(store.getState().messagePrompts.refreshGenerationBySession[SESSION]).toBe(revision);
});

it("invalidates refreshes for live updates outside the prompt projection", () => {
  const store = makeStore();
  const initialRevision = store.getState().messagePrompts.refreshGenerationBySession[SESSION] ?? 0;

  store.getState().updateMessage(message("uncached", "user"));
  store.getState().removeMessage(SESSION, "deleted-before-hydration");

  expect(store.getState().messagePrompts.refreshGenerationBySession[SESSION]).toBe(
    initialRevision + 2,
  );
});
