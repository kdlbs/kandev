import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { fetchAccessibleRepos, GitHubUnavailableError, type AccessibleRepo } from "./github-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<[FetchInput, FetchInit?], Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

function lastCallUrl(): string {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return String(call[0]);
}

describe("fetchAccessibleRepos — URL & parsing", () => {
  it("builds the correct URL with both q and limit", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ repos: [] }));

    await fetchAccessibleRepos({ q: "next", limit: 25 });

    const url = lastCallUrl();
    expect(url).toContain("/api/v1/github/repos");
    expect(url).toContain("q=next");
    expect(url).toContain("limit=25");
  });

  it("omits empty query and missing limit from the URL", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ repos: [] }));

    await fetchAccessibleRepos({});

    const url = lastCallUrl();
    expect(url).toContain("/api/v1/github/repos");
    expect(url).not.toContain("q=");
    expect(url).not.toContain("limit=");
  });

  it("parses the 200 response and injects provider: 'github' on each entry", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        repos: [
          {
            full_name: "kdlbs/kandev",
            owner: "kdlbs",
            name: "kandev",
            private: false,
            pushed_at: "2026-05-20T10:00:00Z",
          },
          {
            full_name: "acme/site",
            owner: "acme",
            name: "site",
            private: true,
          },
        ],
      }),
    );

    const repos: AccessibleRepo[] = await fetchAccessibleRepos({});

    expect(repos).toHaveLength(2);
    expect(repos[0]).toMatchObject({
      provider: "github",
      full_name: "kdlbs/kandev",
      owner: "kdlbs",
      name: "kandev",
      private: false,
      pushed_at: "2026-05-20T10:00:00Z",
    });
    expect(repos[1]).toMatchObject({
      provider: "github",
      full_name: "acme/site",
      owner: "acme",
      name: "site",
      private: true,
    });
    expect(repos[1].pushed_at).toBeUndefined();
  });
});

describe("fetchAccessibleRepos — errors & signal", () => {
  it("throws GitHubUnavailableError on 503 with code: github_not_configured", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: "GitHub is not configured.",
          code: "github_not_configured",
        }),
        { status: 503, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(fetchAccessibleRepos({})).rejects.toBeInstanceOf(GitHubUnavailableError);
  });

  it("throws a plain Error (not GitHubUnavailableError) on 503 without the github_not_configured code", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "transient outage" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const err = await fetchAccessibleRepos({}).catch((e) => e);
    expect(err).toBeInstanceOf(Error);
    expect(err).not.toBeInstanceOf(GitHubUnavailableError);
  });

  it("throws a plain Error on 500", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "boom" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const err = await fetchAccessibleRepos({}).catch((e) => e);
    expect(err).toBeInstanceOf(Error);
    expect(err).not.toBeInstanceOf(GitHubUnavailableError);
  });

  it("forwards AbortSignal: aborting causes the promise to reject", async () => {
    const controller = new AbortController();
    fetchSpy.mockImplementationOnce((_input, init) => {
      return new Promise((_resolve, reject) => {
        const signal = init?.signal;
        if (signal?.aborted) {
          reject(new DOMException("Aborted", "AbortError"));
          return;
        }
        signal?.addEventListener("abort", () => {
          reject(new DOMException("Aborted", "AbortError"));
        });
      });
    });

    const promise = fetchAccessibleRepos({ signal: controller.signal });
    controller.abort();
    await expect(promise).rejects.toThrow();
  });
});
