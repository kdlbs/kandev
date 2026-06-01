import { describe, it, expect } from "vitest";
import type { InfiniteData } from "@tanstack/react-query";
import { flattenTasksPaginated } from "./office";
import type { ListTasksResponse } from "@/lib/api/domains/office-extended-api";
import type { OfficeTask } from "@/lib/state/slices/office/types";

function task(id: string): OfficeTask {
  return {
    id,
    workspaceId: "ws-1",
    identifier: `OFC-${id}`,
    title: `Task ${id}`,
    status: "todo",
    priority: "medium",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

function page(tasks: OfficeTask[] | undefined): ListTasksResponse {
  return { tasks } as ListTasksResponse;
}

function infinite(pages: ListTasksResponse[]): InfiniteData<ListTasksResponse> {
  return { pages, pageParams: pages.map(() => undefined) };
}

describe("flattenTasksPaginated", () => {
  it("returns [] for undefined cache (consumers don't guard)", () => {
    expect(flattenTasksPaginated(undefined)).toEqual([]);
  });

  it("returns [] when there are no pages", () => {
    expect(flattenTasksPaginated(infinite([]))).toEqual([]);
  });

  it("returns the tasks of a single page", () => {
    const result = flattenTasksPaginated(infinite([page([task("1"), task("2")])]));
    expect(result.map((t) => t.id)).toEqual(["1", "2"]);
  });

  it("concatenates tasks across pages in page order", () => {
    const result = flattenTasksPaginated(
      infinite([page([task("1"), task("2")]), page([task("3")]), page([task("4"), task("5")])]),
    );
    expect(result.map((t) => t.id)).toEqual(["1", "2", "3", "4", "5"]);
  });

  it("treats a page with undefined tasks as empty", () => {
    const result = flattenTasksPaginated(
      infinite([page([task("1")]), page(undefined), page([task("2")])]),
    );
    expect(result.map((t) => t.id)).toEqual(["1", "2"]);
  });
});
