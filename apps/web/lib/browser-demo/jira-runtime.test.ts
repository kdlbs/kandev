import { describe, expect, it } from "vitest";
import { DEFAULT_FILTERS, filtersToJql } from "@/components/jira/my-jira/filter-model";
import { createDemoJiraRuntime } from "./jira-runtime";
import type { JiraSearchResult } from "@/lib/types/jira";

describe("demo Jira dashboard", () => {
  it("lists projects and statuses through the worker's current API contracts", () => {
    const runtime = createDemoJiraRuntime();
    expect(runtime.route("/api/v1/jira/projects", "GET", new URLSearchParams(), {})).toMatchObject({
      status: 200,
      body: { projects: [{ key: "PLAT" }] },
    });
    expect(
      runtime.route("/api/v1/jira/projects/PLAT/statuses", "GET", new URLSearchParams(), {}),
    ).toMatchObject({
      status: 200,
      body: { statuses: expect.arrayContaining([expect.objectContaining({ name: "In Review" })]) },
    });
    expect(runtime.route("/api/v1/unhandled", "GET", new URLSearchParams(), {})).toBeNull();
  });

  it("supports the actual dashboard filter builder and escaped search input", () => {
    const runtime = createDemoJiraRuntime();
    const search = (overrides: Partial<typeof DEFAULT_FILTERS>) => {
      const jql = filtersToJql({ ...DEFAULT_FILTERS, ...overrides });
      return runtime.route("/api/v1/jira/tickets", "GET", new URLSearchParams({ jql }), {})
        ?.body as JiraSearchResult;
    };
    expect(search({ projectKeys: ["PLAT"] }).tickets).toHaveLength(4);
    expect(search({ statuses: ["In Review"] }).tickets.map((ticket) => ticket.key)).toEqual([
      "PLAT-142",
    ]);
    expect(search({ searchText: "checkout" }).tickets.map((ticket) => ticket.key)).toEqual([
      "PLAT-137",
    ]);
    expect(search({ searchText: "PLAT-131" }).tickets).toHaveLength(1);
    expect(search({ projectKeys: ["OTHER"] }).tickets).toHaveLength(0);
    expect(search({ assignee: "unassigned" }).tickets).toHaveLength(0);
    expect(search({ searchText: '"quoted" \\ path' }).tickets).toHaveLength(0);
  });

  it("paginates tickets and simulates a transition without contacting Jira", () => {
    const runtime = createDemoJiraRuntime();
    const page = runtime.route(
      "/api/v1/jira/tickets",
      "GET",
      new URLSearchParams({ max_results: "2" }),
      {},
    )?.body as JiraSearchResult;
    expect(page).toMatchObject({ maxResults: 2, isLast: false, nextPageToken: "2" });
    const last = runtime.route(
      "/api/v1/jira/tickets",
      "GET",
      new URLSearchParams({ max_results: "2", page_token: page.nextPageToken! }),
      {},
    )?.body as JiraSearchResult;
    expect(last.isLast).toBe(true);
    expect(last.tickets.map((ticket) => ticket.key)).not.toContain(page.tickets[0].key);
    expect(
      runtime.route("/api/v1/jira/tickets/PLAT-150/transitions", "POST", new URLSearchParams(), {
        transitionId: "review",
      }),
    ).toMatchObject({ status: 200 });
    expect(
      runtime.route("/api/v1/jira/tickets/PLAT-150", "GET", new URLSearchParams(), {}),
    ).toMatchObject({ body: { statusName: "In Review" } });
    expect(
      runtime.route("/api/v1/jira/tickets/PLAT-999", "GET", new URLSearchParams(), {}),
    ).toMatchObject({ status: 404 });
    expect(
      createDemoJiraRuntime().route(
        "/api/v1/jira/tickets/PLAT-150",
        "GET",
        new URLSearchParams(),
        {},
      ),
    ).toMatchObject({ body: { statusName: "Open" } });
  });
});
