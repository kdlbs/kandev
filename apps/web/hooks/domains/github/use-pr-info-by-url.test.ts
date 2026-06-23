import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

const fetchPRInfoMock = vi.fn();
const fetchIssueInfoMock = vi.fn();

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchPRInfo: (...args: unknown[]) => fetchPRInfoMock(...args),
  fetchIssueInfo: (...args: unknown[]) => fetchIssueInfoMock(...args),
}));

// Import after mocks so the hook picks up the mocked module.
import { usePRInfoByURL, parseGitHubIssueUrl, parseGitHubPrUrl } from "./use-pr-info-by-url";

afterEach(() => {
  cleanup();
  fetchPRInfoMock.mockReset();
  fetchIssueInfoMock.mockReset();
  vi.useRealTimers();
});

const PR_URL_A = "https://github.com/acme/site/pull/42";
const PR_URL_B = "https://github.com/acme/api/pull/7";
const ISSUE_URL_A = "https://github.com/acme/site/issues/1456";
const REPO_URL = "https://github.com/acme/site";

function makePR(overrides: { number: number; title?: string; head?: string; base?: string }) {
  return {
    number: overrides.number,
    title: overrides.title ?? "Test PR",
    head_branch: overrides.head ?? "feature/x",
    base_branch: overrides.base ?? "main",
    body: "",
    url: "",
    html_url: "",
    state: "open" as const,
    author_login: "",
    repo_owner: "",
    repo_name: "",
    draft: false,
  };
}

function makeIssue(overrides: { number: number; title?: string; body?: string }) {
  return {
    number: overrides.number,
    title: overrides.title ?? "Test issue",
    body: overrides.body ?? "Issue body",
    url: "",
    html_url: "",
    state: "open" as const,
    author_login: "octocat",
    repo_owner: "acme",
    repo_name: "site",
    labels: [],
    assignees: [],
    created_at: "",
    updated_at: "",
    closed_at: null,
  };
}

describe("parseGitHubPrUrl", () => {
  it("returns owner/repo/prNumber for a canonical PR URL", () => {
    expect(parseGitHubPrUrl(PR_URL_A)).toEqual({
      owner: "acme",
      repo: "site",
      prNumber: 42,
    });
  });
  it("tolerates trailing path/hash (e.g. /files#diff-…)", () => {
    expect(parseGitHubPrUrl(`${PR_URL_A}/files#diff-abc`)).toEqual({
      owner: "acme",
      repo: "site",
      prNumber: 42,
    });
  });
  it("returns null for non-PR URLs (plain repo, invalid, empty)", () => {
    expect(parseGitHubPrUrl(REPO_URL)).toBeNull();
    expect(parseGitHubPrUrl("not a url")).toBeNull();
    expect(parseGitHubPrUrl("")).toBeNull();
  });
});

describe("parseGitHubIssueUrl", () => {
  it("returns owner/repo/issueNumber for a canonical issue URL", () => {
    expect(parseGitHubIssueUrl(ISSUE_URL_A)).toEqual({
      owner: "acme",
      repo: "site",
      issueNumber: 1456,
    });
  });

  it("tolerates trailing path/hash", () => {
    expect(parseGitHubIssueUrl(`${ISSUE_URL_A}#issuecomment-1`)).toEqual({
      owner: "acme",
      repo: "site",
      issueNumber: 1456,
    });
  });

  it("returns null for PR URLs, plain repo URLs, invalid input, and empty input", () => {
    expect(parseGitHubIssueUrl(PR_URL_A)).toBeNull();
    expect(parseGitHubIssueUrl(REPO_URL)).toBeNull();
    expect(parseGitHubIssueUrl("not a url")).toBeNull();
    expect(parseGitHubIssueUrl("")).toBeNull();
  });
});

describe("usePRInfoByURL", () => {
  it("fetches PR info once per unique PR URL when ensure() is called", async () => {
    fetchPRInfoMock.mockImplementation((_o: string, _r: string, n: number) => {
      return Promise.resolve(makePR({ number: n, head: n === 42 ? "feat-a" : "feat-b" }));
    });

    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(PR_URL_A);
      result.current.ensure(PR_URL_B);
    });

    await waitFor(() => {
      expect(result.current.info(PR_URL_A)).toBeDefined();
      expect(result.current.info(PR_URL_B)).toBeDefined();
    });

    expect(fetchPRInfoMock).toHaveBeenCalledTimes(2);
    expect(result.current.info(PR_URL_A)).toMatchObject({
      prHeadBranch: "feat-a",
      prBaseBranch: "main",
      prNumber: 42,
      suggestedTitle: "PR #42: Test PR",
    });
    expect(result.current.info(PR_URL_B)).toMatchObject({
      prHeadBranch: "feat-b",
      prNumber: 7,
    });
  });

  it("dedupes concurrent ensure() calls for the same URL into a single fetch", async () => {
    fetchPRInfoMock.mockResolvedValue(makePR({ number: 42 }));

    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(PR_URL_A);
      result.current.ensure(PR_URL_A);
      result.current.ensure(PR_URL_A);
    });

    await waitFor(() => expect(result.current.info(PR_URL_A)).toBeDefined());
    expect(fetchPRInfoMock).toHaveBeenCalledTimes(1);
  });

  it("reports loading(url) true during fetch and false after settle", async () => {
    let resolveFetch: ((v: ReturnType<typeof makePR>) => void) | null = null;
    fetchPRInfoMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(PR_URL_A);
    });

    await waitFor(() => expect(result.current.loading(PR_URL_A)).toBe(true));

    act(() => {
      resolveFetch?.(makePR({ number: 42 }));
    });

    await waitFor(() => expect(result.current.loading(PR_URL_A)).toBe(false));
    expect(result.current.info(PR_URL_A)).toBeDefined();
  });
});

describe("usePRInfoByURL — non-PR and issue URLs", () => {
  it("no-ops (no fetch, no cached info) for a non-PR repo URL", async () => {
    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(REPO_URL);
    });

    // Nothing to wait for — the call should be synchronous. Assert no fetch.
    expect(fetchPRInfoMock).not.toHaveBeenCalled();
    expect(result.current.info(REPO_URL)).toBeUndefined();
    expect(result.current.loading(REPO_URL)).toBe(false);

    // Repeated ensure() for the same non-PR URL also no-ops (dedup via loadedRef).
    act(() => {
      result.current.ensure(REPO_URL);
    });
    expect(fetchPRInfoMock).not.toHaveBeenCalled();
  });

  it("clear() is a no-op after ensure() records a non-GitHub URL as loaded", () => {
    const url = "https://example.com/not-github";
    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(url);
      result.current.clear(url);
    });

    expect(fetchPRInfoMock).not.toHaveBeenCalled();
    expect(fetchIssueInfoMock).not.toHaveBeenCalled();
    expect(result.current.info(url)).toBeUndefined();
    expect(result.current.loading(url)).toBe(false);
  });

  it("fetches issue info and exposes a suggested title for GitHub issue URLs", async () => {
    fetchIssueInfoMock.mockResolvedValue(makeIssue({ number: 1456, title: "Fix remote picker" }));

    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(ISSUE_URL_A);
    });

    await waitFor(() => expect(result.current.info(ISSUE_URL_A)).toBeDefined());
    expect(fetchIssueInfoMock).toHaveBeenCalledTimes(1);
    expect(fetchIssueInfoMock).toHaveBeenCalledWith("acme", "site", 1456, expect.any(Object));
    expect(fetchPRInfoMock).not.toHaveBeenCalled();
    expect(result.current.info(ISSUE_URL_A)).toMatchObject({
      issueNumber: 1456,
      suggestedTitle: "Issue #1456: Fix remote picker",
    });
  });

  it("ignores ensure() with empty string", () => {
    const { result } = renderHook(() => usePRInfoByURL());
    act(() => {
      result.current.ensure("");
    });
    expect(fetchPRInfoMock).not.toHaveBeenCalled();
  });
});

describe("usePRInfoByURL — cache invalidation", () => {
  it("does not re-fetch when ensure() is called for an already-loaded URL", async () => {
    fetchPRInfoMock.mockResolvedValue(makePR({ number: 42 }));

    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(PR_URL_A);
    });
    await waitFor(() => expect(result.current.info(PR_URL_A)).toBeDefined());

    act(() => {
      result.current.ensure(PR_URL_A);
    });

    expect(fetchPRInfoMock).toHaveBeenCalledTimes(1);
  });

  it("clear(url) forgets the cached entry so the next ensure() re-fetches", async () => {
    fetchPRInfoMock.mockResolvedValue(makePR({ number: 42 }));
    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(PR_URL_A);
    });
    await waitFor(() => expect(result.current.info(PR_URL_A)).toBeDefined());
    expect(fetchPRInfoMock).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.clear(PR_URL_A);
    });
    expect(result.current.info(PR_URL_A)).toBeUndefined();

    act(() => {
      result.current.ensure(PR_URL_A);
    });
    await waitFor(() => expect(fetchPRInfoMock).toHaveBeenCalledTimes(2));
  });

  it("returns undefined info / false loading for an unknown URL", () => {
    const { result } = renderHook(() => usePRInfoByURL());
    expect(result.current.info("https://github.com/who/what/pull/1")).toBeUndefined();
    expect(result.current.loading("https://github.com/who/what/pull/1")).toBe(false);
  });

  it("ignores a stale fetch resolved after clear() + ensure() restarted the request", async () => {
    // Simulates the race the per-URL sequence counter guards: the first
    // fetch is still in flight when the caller clear()s and re-ensure()s;
    // the SECOND fetch resolves first; the FIRST (stale) fetch then
    // resolves and must NOT clobber the state.
    let resolveFirst: ((v: ReturnType<typeof makePR>) => void) | null = null;
    let resolveSecond: ((v: ReturnType<typeof makePR>) => void) | null = null;
    fetchPRInfoMock
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveSecond = resolve;
          }),
      );

    const { result } = renderHook(() => usePRInfoByURL());

    act(() => {
      result.current.ensure(PR_URL_A);
    });
    await waitFor(() => expect(fetchPRInfoMock).toHaveBeenCalledTimes(1));

    // Clear forgets the cached entry AND aborts the in-flight request;
    // immediately re-ensure() to kick off a fresh request for the same URL.
    act(() => {
      result.current.clear(PR_URL_A);
      result.current.ensure(PR_URL_A);
    });
    await waitFor(() => expect(fetchPRInfoMock).toHaveBeenCalledTimes(2));

    // Second fetch resolves first with the up-to-date PR (title "Fresh").
    act(() => {
      resolveSecond?.(makePR({ number: 99, title: "Fresh" }));
    });
    await waitFor(() => expect(result.current.info(PR_URL_A)?.prNumber).toBe(99));

    // Now the stale first fetch resolves — its callback must observe that
    // its sequence number is no longer current and bail without overwriting
    // the fresh state.
    act(() => {
      resolveFirst?.(makePR({ number: 42, title: "Stale" }));
    });
    // Give the microtask queue a chance to drain.
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.info(PR_URL_A)?.prNumber).toBe(99);
    expect(result.current.info(PR_URL_A)?.suggestedTitle).toBe("PR #99: Fresh");
  });
});
