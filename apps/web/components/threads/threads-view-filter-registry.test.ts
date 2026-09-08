import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import { getThreadFilterOptions, THREAD_SORT_OPTIONS } from "./threads-view-filter-registry";

const candidate = {
  taskId: "task-1",
  repositoryIds: ["repository-1"],
  taskState: "IN_PROGRESS",
  priority: "high",
  taskOrigin: "agent_created",
  executorType: "local_docker",
} as ThreadCandidate;

const translate = ((key: string) => key) as unknown as TFunction;

describe("Threads filter option labels", () => {
  it("keeps stable identifiers as values while resolving display labels", () => {
    const repositoryNames = new Map([["repository-1", "Kandev"]]);

    expect(getThreadFilterOptions("repository", [candidate], translate, repositoryNames)).toEqual([
      { value: "repository-1", label: "Kandev" },
    ]);
    expect(getThreadFilterOptions("taskState", [candidate], translate)).toEqual([
      { value: "IN_PROGRESS", label: "task:statusInProgress" },
    ]);
    expect(getThreadFilterOptions("priority", [candidate], translate)).toEqual([
      { value: "high", label: "task:priorityHigh" },
    ]);
    expect(getThreadFilterOptions("taskOrigin", [candidate], translate)).toEqual([
      { value: "agent_created", label: "threads:originAgentCreated" },
    ]);
  });
});

describe("Threads sort options", () => {
  it("gives every sort option a localized explanation", () => {
    expect(THREAD_SORT_OPTIONS).toHaveLength(9);
    expect(
      THREAD_SORT_OPTIONS.every((option) => {
        const descriptionKey = (option as { descriptionKey?: unknown }).descriptionKey;
        return typeof descriptionKey === "string" && descriptionKey.startsWith("threads:");
      }),
    ).toBe(true);
  });
});
