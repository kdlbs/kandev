import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { requestPRReviewers, submitPRReview } from "./github-review-api";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("PR review writes", () => {
  it("posts review events and an empty default body to the reviews endpoint", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ submitted: true }));

    await expect(submitPRReview("acme", "site", 42, "APPROVE")).resolves.toEqual({
      submitted: true,
    });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe("http://api.test/api/v1/github/prs/acme/site/42/reviews");
    expect(call?.[1]?.method).toBe("POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ event: "APPROVE", body: "" });
  });

  it("posts reviewer logins to the pull request review-request endpoint", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ requested: true }));

    await expect(
      requestPRReviewers("acme", "site", 42, ["octocat"], "workspace-1"),
    ).resolves.toEqual({
      requested: true,
    });

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe(
      "http://api.test/api/v1/github/prs/acme/site/42/requested-reviewers?workspace_id=workspace-1",
    );
    expect(call?.[1]?.method).toBe("POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ reviewers: ["octocat"] });
  });
});
