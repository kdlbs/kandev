import type { TFunction } from "i18next";
import { describe, expect, it } from "vitest";
import type { RepositoryId } from "@/lib/types/ids";
import {
  buildDisplayBooleanSummary,
  buildDisplayFilterSummary,
  buildDisplaySortSummary,
} from "./display-settings-summary";

const t = ((key: string, options?: { count?: number; label?: string }) => {
  const labels: Record<string, string> = {
    "common:loading": "Loading",
    "common:unavailable": "Unavailable",
    "kanban:allPriorities": "All priorities",
    "kanban:allRepositories": "All repositories",
    "kanban:allWorkflowsTitleCase": "All Workflows",
    "kanban:boardSortCreatedDesc": "Newest first",
    "kanban:boardSortPriorityDesc": "Priority",
    "kanban:noRepositories": "No repositories",
    "kanban:off": "Off",
    "kanban:on": "On",
    "kanban:priorityCritical": "Critical",
    "kanban:priorityHigh": "High",
    "kanban:priorityLow": "Low",
    "kanban:priorityMedium": "Medium",
  };
  if (key === "kanban:selectedFilterCount") {
    return `${options?.label}: ${options?.count} selected`;
  }
  return labels[key] ?? key;
}) as TFunction;

const baseOptions = {
  t,
  activeWorkflowId: null,
  workflows: [],
  showWorkflow: true,
  repositoryValue: "all",
  repositories: [],
  repositoriesLoading: false,
  showRepository: false,
  priorityFilterTokens: [],
  showPriority: true,
};

describe("display settings summaries", () => {
  it("uses explicit All summaries for empty selections", () => {
    expect(buildDisplayFilterSummary(baseOptions)).toBe("All Workflows, All priorities");
  });

  it("uses loading and unavailable states without inventing names", () => {
    expect(
      buildDisplayFilterSummary({
        ...baseOptions,
        activeWorkflowId: "missing-workflow",
        repositoryValue: "all",
        repositoriesLoading: true,
        showRepository: true,
      }),
    ).toBe("Unavailable, Loading, All priorities");

    expect(
      buildDisplayFilterSummary({
        ...baseOptions,
        activeWorkflowId: "missing-workflow",
        repositoryValue: "missing-repository",
        showRepository: true,
      }),
    ).toBe("Unavailable, Unavailable, All priorities");
  });

  it("lists selected priorities and active plugin counts", () => {
    expect(
      buildDisplayFilterSummary({
        ...baseOptions,
        workflows: [{ id: "wf-a", workspaceId: "workspace-a", name: "Workflow A" }],
        activeWorkflowId: "wf-a",
        repositories: [{ id: "repo-a" as RepositoryId, name: "Repository A" }],
        repositoryValue: "repo-a",
        showRepository: true,
        priorityFilterTokens: ["critical", "high", "medium", "low"],
        pluginFilters: [
          {
            pluginId: "plugin-a",
            id: "tags",
            label: "Tags",
            getOptions: () => [],
            matches: () => true,
          },
        ],
        pluginFilterSelections: { "plugin-a:tags": ["bug", "feature"] },
      }),
    ).toBe("Workflow A, Repository A, Critical, High, Medium, Low, Tags: 2 selected");
  });

  it("summarizes sort and boolean values through translation keys", () => {
    expect(buildDisplaySortSummary(t, "created_desc")).toBe("Newest first");
    expect(buildDisplaySortSummary(t, "priority_desc")).toBe("Priority");
    expect(buildDisplayBooleanSummary(t, true)).toBe("On");
    expect(buildDisplayBooleanSummary(t, false)).toBe("Off");
  });
});
