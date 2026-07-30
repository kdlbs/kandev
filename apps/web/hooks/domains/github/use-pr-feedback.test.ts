import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { PRFeedback } from "@/lib/types/github";
import { resolvePRFeedbackView, usePRFeedback, type PRFeedbackState } from "./use-pr-feedback";

const getPRFeedbackMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/github-api", () => ({
  getPRFeedback: getPRFeedbackMock,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function makeFeedback(commentBody: string): PRFeedback {
  return {
    pr: {
      number: 1,
      title: "Test PR",
      url: "https://github.com/acme/app/pull/1",
      html_url: "https://github.com/acme/app/pull/1",
      state: "open",
      head_branch: "feat",
      base_branch: "main",
      author_login: "alice",
      repo_owner: "acme",
      repo_name: "app",
      draft: false,
      mergeable: true,
      additions: 1,
      deletions: 0,
      requested_reviewers: [],
      created_at: "",
      updated_at: "",
      merged_at: null,
      closed_at: null,
    },
    reviews: [],
    comments: [
      {
        id: 1,
        author: "bot",
        author_avatar: "",
        author_is_bot: true,
        body: commentBody,
        path: "",
        line: 0,
        side: "",
        comment_type: "issue",
        created_at: "",
        updated_at: "",
        in_reply_to: null,
      },
    ],
    checks: [],
    has_issues: false,
  };
}

beforeEach(() => {
  getPRFeedbackMock.mockReset();
});

function wrapper({ children }: { children: ReactNode }) {
  const initialState = {
    workspaces: { activeId: "ws-1" },
  } as unknown as Partial<AppState>;
  return createElement(StateProvider, { initialState, children });
}

const staleState: PRFeedbackState = {
  key: "ws-1/acme/app/1",
  feedback: makeFeedback("stale comment from PR 1"),
  loading: false,
  error: null,
};

describe("resolvePRFeedbackView", () => {
  it("masks the previous PR's feedback while a newly requested PR starts loading", () => {
    expect(resolvePRFeedbackView(staleState, "ws-1/acme/app/2")).toEqual({
      feedback: null,
      loading: true,
      error: null,
    });
  });

  it("clears stale feedback without loading when no PR is requested", () => {
    expect(resolvePRFeedbackView(staleState, "")).toEqual({
      feedback: null,
      loading: false,
      error: null,
    });
  });

  it("passes through cached feedback when the requested key matches (e.g. manual refresh)", () => {
    expect(resolvePRFeedbackView(staleState, "ws-1/acme/app/1")).toEqual({
      feedback: staleState.feedback,
      loading: false,
      error: null,
    });
  });
});

describe("usePRFeedback request ownership", () => {
  it("masks PR 1's feedback immediately on switching to PR 2, and ignores PR 1 when it resolves late", async () => {
    const first = deferred<PRFeedback>();
    const second = deferred<PRFeedback>();
    getPRFeedbackMock.mockImplementation(
      (_ws: string, _owner: string, _repo: string, prNumber: number) =>
        prNumber === 1 ? first.promise : second.promise,
    );

    const { result, rerender } = renderHook(
      ({ prNumber }) => usePRFeedback("ws-1", "acme", "app", prNumber),
      { initialProps: { prNumber: 1 }, wrapper },
    );

    await act(async () => {
      first.resolve(makeFeedback("PR 1 comment"));
      await first.promise;
    });
    await waitFor(() => expect(result.current.feedback?.comments[0]?.body).toBe("PR 1 comment"));

    // Switch to a different PR (simulating switching tasks) before the new
    // fetch resolves — the panel must not keep showing PR 1's stale content.
    rerender({ prNumber: 2 });
    expect(result.current.feedback).toBeNull();
    expect(result.current.loading).toBe(true);

    await act(async () => {
      second.resolve(makeFeedback("PR 2 comment"));
      await second.promise;
    });
    await waitFor(() => expect(result.current.feedback?.comments[0]?.body).toBe("PR 2 comment"));

    // PR 1's request resolving late must not resurrect its stale data.
    expect(result.current.feedback?.comments[0]?.body).toBe("PR 2 comment");
  });

  it("keeps showing cached feedback while a manual refresh of the same PR is in flight", async () => {
    const first = deferred<PRFeedback>();
    getPRFeedbackMock.mockReturnValueOnce(first.promise);

    const { result } = renderHook(() => usePRFeedback("ws-1", "acme", "app", 1), { wrapper });

    await act(async () => {
      first.resolve(makeFeedback("initial comment"));
      await first.promise;
    });
    await waitFor(() => expect(result.current.feedback?.comments[0]?.body).toBe("initial comment"));

    const second = deferred<PRFeedback>();
    getPRFeedbackMock.mockReturnValueOnce(second.promise);
    act(() => result.current.refresh());

    // Same identity refresh: old feedback stays visible while reloading.
    expect(result.current.feedback?.comments[0]?.body).toBe("initial comment");
    expect(result.current.loading).toBe(true);
  });
});
