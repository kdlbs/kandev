import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useForgejoPullRequestDetails } from "./use-forgejo-pull-request-details";

const api = vi.hoisted(() => ({ details: vi.fn(), comment: vi.fn(), review: vi.fn() }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({ getForgejoPullRequestDetails: api.details, createForgejoPullRequestComment: api.comment, submitForgejoPullRequestReview: api.review }));

const details = { owner: "owner", repo: "repo", pull_request: { number: 7 }, commits: [], files: [], comments: [], reviews: [], action_runs: [] };

describe("useForgejoPullRequestDetails", () => {
  beforeEach(() => { vi.clearAllMocks(); api.details.mockResolvedValue(details); });
  it("loads details for the selected pull request", async () => {
    const { result } = renderHook(() => useForgejoPullRequestDetails("workspace-a"));
    await act(() => result.current.load("owner", "repo", 7));
    expect(api.details).toHaveBeenCalledWith("owner", "repo", 7, { workspaceId: "workspace-a" });
    expect(result.current.details).toEqual(details);
  });
  it("posts feedback then refreshes details", async () => {
    api.comment.mockResolvedValue({ id: 1 });
    const { result } = renderHook(() => useForgejoPullRequestDetails("workspace-a"));
    await act(() => result.current.comment("owner", "repo", 7, "looks good"));
    await waitFor(() => expect(api.comment).toHaveBeenCalledWith({ owner: "owner", repo: "repo", number: 7, body: "looks good" }, { workspaceId: "workspace-a" }));
    expect(api.details).toHaveBeenCalledWith("owner", "repo", 7, { workspaceId: "workspace-a" });
  });
});
