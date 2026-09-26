import { afterEach, describe, it, expect } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { applyGroup, applyView } from "./apply-view";
import { i18n } from "@/lib/i18n";
import type { TaskState } from "@/lib/types/http";

function task(overrides: Partial<TaskSwitcherItem>): TaskSwitcherItem {
  return { id: overrides.id ?? "t", title: overrides.title ?? "Task", ...overrides };
}

const ALPHA = "/repos/alpha";
const BETA = "/repos/beta";

/** Two repos keep the `__unassigned__` bucket; one repo merges it away. */
const REPO_TASKS = [
  task({ id: "a", repositoryPath: ALPHA }),
  task({ id: "b", repositoryPath: BETA }),
  task({ id: "c" }),
  task({ id: "d", repositories: [ALPHA, BETA] }),
];

function labelFor(groups: { key: string; label: string }[], key: string): string | undefined {
  return groups.find((g) => g.key === key)?.label;
}

afterEach(async () => {
  await i18n.changeLanguage("en");
});

describe("applyGroup — group labels are catalog-backed", () => {
  it("labels the repository sentinel buckets in English", () => {
    const { groups } = applyGroup(REPO_TASKS, "repository");
    expect(labelFor(groups, "__multi__")).toBe("Multi-repo");
    expect(labelFor(groups, "__unassigned__")).toBe("Unassigned");
  });

  it("labels the ungrouped bucket", () => {
    const { groups } = applyGroup([task({ id: "a" })], "none");
    expect(groups).toHaveLength(1);
    expect(groups[0].key).toBe("__all__");
    expect(groups[0].label).toBe("All");
  });

  it.each(["workflow", "workflowStep", "executorType"] as const)(
    "labels the %s bucket for a task with no value",
    (groupKey) => {
      const { groups } = applyGroup([task({ id: "a" })], groupKey);
      expect(labelFor(groups, "__unassigned__")).toBe("Unassigned");
    },
  );

  it("uses the task-state vocabulary for state group headings", () => {
    const { groups } = applyGroup(
      [task({ id: "a", state: "IN_PROGRESS" }), task({ id: "b" })],
      "state",
    );
    expect(labelFor(groups, "IN_PROGRESS")).toBe("In progress");
    expect(labelFor(groups, "__not_started__")).toBe("Not started");
  });

  it("echoes an unmapped task state rather than rendering a blank heading", () => {
    const { groups } = applyGroup(
      [task({ id: "a", state: "UNKNOWN_FUTURE" as TaskState })],
      "state",
    );
    expect(labelFor(groups, "UNKNOWN_FUTURE")).toBe("UNKNOWN_FUTURE");
  });

  it("translates labels on a locale switch while group keys stay identity", async () => {
    await i18n.changeLanguage("pseudo");
    const { groups } = applyGroup(REPO_TASKS, "repository");
    // Keys are compared in mergeSingleRepoUnassigned/sortRepoGroups and
    // persisted in the view's collapsedGroups — they must never localize.
    expect(groups.map((g) => g.key).sort()).toEqual([ALPHA, BETA, "__multi__", "__unassigned__"]);
    expect(labelFor(groups, "__multi__")).not.toBe("Multi-repo");
    expect(labelFor(groups, "__multi__")).toMatch(/[^\p{ASCII}]/u);
    expect(labelFor(groups, "__unassigned__")).toMatch(/[^\p{ASCII}]/u);
    // A repository path is user data, not copy.
    expect(labelFor(groups, ALPHA)).toBe(ALPHA);
  });

  it("translates the ungrouped label on a locale switch", async () => {
    await i18n.changeLanguage("pseudo");
    const { groups } = applyGroup([task({ id: "a" })], "none");
    expect(groups[0].key).toBe("__all__");
    expect(groups[0].label).not.toBe("All");
    expect(groups[0].label).toMatch(/[^\p{ASCII}]/u);
  });
});

describe("applyGroup — grouping dimension is reported back", () => {
  // Rows read `groupKey` to decide whether naming their own repository would
  // just repeat the section header, so the grouped list has to carry it.
  it("reports the dimension it grouped by", () => {
    expect(applyGroup(REPO_TASKS, "repository").groupKey).toBe("repository");
    expect(applyGroup(REPO_TASKS, "workflowStep").groupKey).toBe("workflowStep");
  });

  it("reports an ungrouped flat list as such", () => {
    expect(applyGroup(REPO_TASKS, "none").groupKey).toBe("none");
  });

  // applyView post-processes the grouped list (pinning, subtask order); the
  // dimension has to survive that trip, since the rows read it from there.
  it("survives applyView's post-processing", () => {
    const view = {
      filters: [],
      sort: { key: "createdAt", direction: "desc" },
      group: "repository",
      collapsedGroups: [],
    } as unknown as Parameters<typeof applyView>[1];

    expect(applyView(REPO_TASKS, view, { pinnedTaskIds: ["a"], orderedTaskIds: [] }).groupKey).toBe(
      "repository",
    );
  });
});
