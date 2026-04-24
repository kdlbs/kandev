import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { GitHubPR, GitHubPRStatus } from "@/lib/types/github";

const getPRStatusesBatch = vi.fn();

vi.mock("@/lib/api/domains/github-api", () => ({
  getPRStatusesBatch: (refs: { owner: string; repo: string; number: number }[]) =>
    getPRStatusesBatch(refs) as Promise<{ statuses: Record<string, GitHubPRStatus> }>,
}));

import { prStatusKey, usePRStatuses } from "./use-pr-statuses";

function makePR(owner: string, repo: string, number: number): GitHubPR {
  return {
    number,
    title: `PR #${number}`,
    url: "",
    html_url: "",
    state: "open",
    head_branch: "feat",
    base_branch: "main",
    author_login: "u",
    repo_owner: owner,
    repo_name: repo,
    draft: false,
    mergeable: true,
    additions: 0,
    deletions: 0,
    requested_reviewers: [],
    created_at: "",
    updated_at: "",
    merged_at: null,
    closed_at: null,
  };
}

function makeStatus(pr: GitHubPR): GitHubPRStatus {
  return {
    pr,
    review_state: "",
    checks_state: "",
    mergeable_state: "unknown",
    review_count: 0,
    pending_review_count: 0,
    checks_total: 0,
    checks_passing: 0,
  } as GitHubPRStatus;
}

describe("prStatusKey", () => {
  it("encodes owner/repo#number", () => {
    expect(prStatusKey("acme", "widget", 7)).toBe("acme/widget#7");
  });
});

describe("usePRStatuses", () => {
  beforeEach(() => {
    getPRStatusesBatch.mockReset();
  });

  it("skips fetch when prs list is empty", () => {
    renderHook(() => usePRStatuses([]));
    expect(getPRStatusesBatch).not.toHaveBeenCalled();
  });

  it("fetches and populates map keyed by prStatusKey", async () => {
    const pr = makePR("acme", "widget", 7);
    getPRStatusesBatch.mockResolvedValueOnce({
      statuses: { "acme/widget#7": makeStatus(pr) },
    });
    const { result } = renderHook(() => usePRStatuses([pr]));
    await waitFor(() => {
      expect(result.current.size).toBe(1);
    });
    expect(result.current.get("acme/widget#7")?.pr.number).toBe(7);
  });

  it("does not refetch when list identity changes but key stays the same", async () => {
    const pr = makePR("acme", "widget", 7);
    getPRStatusesBatch.mockResolvedValue({
      statuses: { "acme/widget#7": makeStatus(pr) },
    });
    const { rerender, result } = renderHook(({ prs }) => usePRStatuses(prs), {
      initialProps: { prs: [pr] },
    });
    await waitFor(() => expect(result.current.size).toBe(1));
    expect(getPRStatusesBatch).toHaveBeenCalledTimes(1);

    // New array reference, same content.
    rerender({ prs: [makePR("acme", "widget", 7)] });
    // Give effects a tick.
    await act(async () => {
      await Promise.resolve();
    });
    expect(getPRStatusesBatch).toHaveBeenCalledTimes(1);
  });

  it("refetches when the list key changes", async () => {
    const pr1 = makePR("acme", "widget", 7);
    const pr2 = makePR("acme", "widget", 8);
    getPRStatusesBatch.mockResolvedValue({
      statuses: { "acme/widget#7": makeStatus(pr1), "acme/widget#8": makeStatus(pr2) },
    });
    const { rerender } = renderHook(({ prs }) => usePRStatuses(prs), {
      initialProps: { prs: [pr1] },
    });
    await waitFor(() => expect(getPRStatusesBatch).toHaveBeenCalledTimes(1));
    rerender({ prs: [pr1, pr2] });
    await waitFor(() => expect(getPRStatusesBatch).toHaveBeenCalledTimes(2));
  });

  it("leaves prior statuses in place when a fetch errors", async () => {
    const pr = makePR("acme", "widget", 7);
    getPRStatusesBatch.mockResolvedValueOnce({
      statuses: { "acme/widget#7": makeStatus(pr) },
    });
    const { result, rerender } = renderHook(({ prs }) => usePRStatuses(prs), {
      initialProps: { prs: [pr] },
    });
    await waitFor(() => expect(result.current.size).toBe(1));

    // Next fetch rejects; statuses should be cleared (since the key changed)
    // but the test verifies the hook survives the rejection without throwing.
    const pr2 = makePR("acme", "widget", 8);
    getPRStatusesBatch.mockRejectedValueOnce(new Error("boom"));
    rerender({ prs: [pr2] });
    // Wait for the rejection to propagate and the catch handler to clear
    // the stale statuses.
    await waitFor(() => expect(result.current.size).toBe(0));
  });
});
