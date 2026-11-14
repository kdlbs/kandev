import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { AuthSliceState } from "@/lib/state/slices/auth/types";
import {
  MAX_RETAINED_ANSWER_DRAFTS,
  answerDraftAuthorityKey,
  answerDraftGeneration,
  canRetainAnswerDrafts,
  clearAnswerDrafts,
  readAnswerDraft,
  retireAnswerDraft,
  writeAnswerDraft,
} from "./answer-draft-cache";

type AuthSnapshot = AuthSliceState["auth"];

const BUNDLE = "bundle";
function identity(taskId: string, bundleId = BUNDLE) {
  return JSON.stringify([taskId, bundleId]);
}
function draft(text: string) {
  return {
    customDrafts: { q1: text },
    activeIndex: 1,
    answers: [{ question_id: "q2", selected_options: ["yes"] }],
  };
}
function user(id: string, role = "member", status = "active") {
  return { id, email: `${id}@example.com`, display_name: id, role, status };
}
function enabledAuth(authenticated: boolean, id = "user-1"): AuthSnapshot {
  return {
    mode: "enabled",
    authenticated,
    user: authenticated ? user(id) : null,
  };
}

describe("answer draft cache", () => {
  it("isolates drafts by store owner", () => {
    const first = createAppStore();
    const second = createAppStore();
    expect(
      writeAnswerDraft(first, identity("task"), answerDraftGeneration(first), draft("keep")),
    ).toBe(true);

    expect(readAnswerDraft(first, identity("task"))).toEqual(draft("keep"));
    expect(readAnswerDraft(second, identity("task"))).toBeUndefined();
  });

  it("isolates drafts by task and bundle identity", () => {
    const store = createAppStore();
    const generation = answerDraftGeneration(store);
    writeAnswerDraft(store, identity("task-a"), generation, draft("keep"));

    expect(readAnswerDraft(store, identity("task-b"))).toBeUndefined();
    expect(readAnswerDraft(store, identity("task-a", "other-bundle"))).toBeUndefined();
    expect(readAnswerDraft(store, identity("task-a"))).toEqual(draft("keep"));
  });

  it("clears drafts and fences captured generations on an auth change without a form", () => {
    const store = createAppStore();
    const generation = answerDraftGeneration(store);
    writeAnswerDraft(store, identity("task"), generation, draft("keep"));

    store.getState().setAuthState(enabledAuth(true));

    expect(readAnswerDraft(store, identity("task"))).toBeUndefined();
    expect(answerDraftGeneration(store)).toBe(generation + 1);
    expect(writeAnswerDraft(store, identity("task"), generation, draft("stale"))).toBe(false);
    expect(readAnswerDraft(store, identity("task"))).toBeUndefined();
  });

  it("does not restore cleared drafts when the same user returns", () => {
    const store = createAppStore();
    store.getState().setAuthState(enabledAuth(true, "user-1"));
    writeAnswerDraft(store, identity("task"), answerDraftGeneration(store), draft("keep"));

    store.getState().setAuthState(enabledAuth(false));
    store.getState().setAuthState(enabledAuth(true, "user-1"));

    expect(readAnswerDraft(store, identity("task"))).toBeUndefined();
  });

  it("refuses stale settlement so a delayed callback cannot remove a newer draft", () => {
    const store = createAppStore();
    const staleGeneration = answerDraftGeneration(store);
    writeAnswerDraft(store, identity("task"), staleGeneration, draft("keep"));
    clearAnswerDrafts(store);
    const currentGeneration = answerDraftGeneration(store);
    writeAnswerDraft(store, identity("task"), currentGeneration, draft("new"));

    expect(retireAnswerDraft(store, identity("task"), staleGeneration)).toBe(false);
    expect(readAnswerDraft(store, identity("task"))).toEqual(draft("new"));
    expect(retireAnswerDraft(store, identity("task"), currentGeneration)).toBe(true);
    expect(readAnswerDraft(store, identity("task"))).toBeUndefined();
  });

  it("bounds retained bundles and evicts the least recently used draft", () => {
    const store = createAppStore();
    const generation = answerDraftGeneration(store);
    for (let index = 0; index < MAX_RETAINED_ANSWER_DRAFTS; index += 1) {
      writeAnswerDraft(store, identity(`task-${index}`), generation, draft(`${index}`));
    }

    expect(readAnswerDraft(store, identity("task-0"))).toEqual(draft("0"));
    writeAnswerDraft(store, identity("task-8"), generation, draft("8"));

    expect(readAnswerDraft(store, identity("task-0"))).toEqual(draft("0"));
    expect(readAnswerDraft(store, identity("task-1"))).toBeUndefined();
    expect(readAnswerDraft(store, identity("task-2"))).toEqual(draft("2"));
    expect(readAnswerDraft(store, identity("task-8"))).toEqual(draft("8"));
  });

  it("does not refresh recency for an unchanged write", () => {
    const store = createAppStore();
    const generation = answerDraftGeneration(store);
    for (let index = 0; index < MAX_RETAINED_ANSWER_DRAFTS; index += 1) {
      writeAnswerDraft(store, identity(`task-${index}`), generation, draft(`${index}`));
    }

    writeAnswerDraft(store, identity("task-0"), generation, draft("0"));
    writeAnswerDraft(store, identity("task-8"), generation, draft("8"));

    expect(readAnswerDraft(store, identity("task-0"))).toBeUndefined();
    expect(readAnswerDraft(store, identity("task-1"))).toEqual(draft("1"));
  });

  it("refreshes recency on an edit", () => {
    const store = createAppStore();
    const generation = answerDraftGeneration(store);
    for (let index = 0; index < MAX_RETAINED_ANSWER_DRAFTS; index += 1) {
      writeAnswerDraft(store, identity(`task-${index}`), generation, draft(`${index}`));
    }

    writeAnswerDraft(store, identity("task-0"), generation, draft("edited"));
    writeAnswerDraft(store, identity("task-8"), generation, draft("8"));

    expect(readAnswerDraft(store, identity("task-0"))).toEqual(draft("edited"));
    expect(readAnswerDraft(store, identity("task-1"))).toBeUndefined();
  });
});

describe("answer draft authority", () => {
  it("tracks mode, authentication, user, role, and status", () => {
    const base = enabledAuth(true);
    const keys = new Set([
      answerDraftAuthorityKey(base),
      answerDraftAuthorityKey({ ...base, mode: "disabled" }),
      answerDraftAuthorityKey({ ...base, authenticated: false, user: null }),
      answerDraftAuthorityKey({ ...base, user: user("user-2") }),
      answerDraftAuthorityKey({ ...base, user: user("user-1", "admin") }),
      answerDraftAuthorityKey({ ...base, user: user("user-1", "member", "disabled") }),
    ]);

    expect(keys.size).toBe(6);
  });

  it("allows local auth-disabled stores and authenticated sessions only", () => {
    expect(canRetainAnswerDrafts({ mode: "disabled", authenticated: true, user: null })).toBe(true);
    expect(canRetainAnswerDrafts(enabledAuth(true))).toBe(true);
    expect(canRetainAnswerDrafts(enabledAuth(false))).toBe(false);
    expect(canRetainAnswerDrafts({ mode: "setup", authenticated: false, user: null })).toBe(true);
  });

  it("stores annotation-only changes to an existing draft", () => {
    const store = createAppStore();
    const generation = answerDraftGeneration(store);
    const region = {
      image_id: "img-1",
      x: 1,
      y: 2,
      width: 3,
      height: 4,
      image_width: 10,
      image_height: 10,
      comment: "first",
    };
    writeAnswerDraft(store, identity("task"), generation, {
      ...draft("keep"),
      annotations: { q1: [region] },
    });

    writeAnswerDraft(store, identity("task"), generation, {
      ...draft("keep"),
      annotations: { q1: [{ ...region, comment: "edited" }] },
    });
    expect(readAnswerDraft(store, identity("task"))?.annotations?.q1?.[0]?.comment).toBe("edited");

    writeAnswerDraft(store, identity("task"), generation, { ...draft("keep"), annotations: {} });
    expect(readAnswerDraft(store, identity("task"))?.annotations).toEqual({});
  });
});
