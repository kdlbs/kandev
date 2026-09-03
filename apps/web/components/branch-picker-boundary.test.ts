import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const componentsDirectory = dirname(fileURLToPath(import.meta.url));

function readComponent(relativePath: string): string {
  return readFileSync(join(componentsDirectory, relativePath), "utf8");
}

describe("branch picker module boundaries", () => {
  it("keeps shared branch picker code out of task-create-owned modules", () => {
    const selectorPath = join(componentsDirectory, "branch-selector.tsx");
    const optionsPath = join(componentsDirectory, "branch-picker-options.tsx");
    expect(existsSync(selectorPath)).toBe(true);
    expect(existsSync(optionsPath)).toBe(true);

    const sharedSources = [
      readComponent("branch-selector.tsx"),
      readComponent("branch-picker-options.tsx"),
    ];
    for (const source of sharedSources) {
      expect(source).not.toContain("task-create-dialog");
    }

    expect(readComponent("watcher-repository-fields.tsx")).not.toContain(
      "task-create-dialog-selectors",
    );
    expect(readComponent("watcher-repository-fields.tsx")).not.toContain(
      "task-create-dialog-branch-options",
    );
    for (const consumer of [
      "settings/repository-branch-policy-fields.tsx",
      "settings/repository-branch-policies.tsx",
      "task-create-dialog-options.tsx",
      "task-create-dialog-pill.test.tsx",
      "task-create-dialog-repo-chip-parts.tsx",
      "task-create-dialog-remote-repo-chip.tsx",
      "task-create-dialog-workspace-repo-chips.tsx",
    ]) {
      expect(readComponent(consumer)).not.toContain("task-create-dialog-branch-options");
    }
  });
});
