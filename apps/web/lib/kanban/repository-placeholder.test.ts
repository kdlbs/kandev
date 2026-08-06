import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import { getRepositoryPlaceholderKey } from "./repository-placeholder";

describe("getRepositoryPlaceholderKey", () => {
  it("prefers the loading state even when the list is still empty", () => {
    expect(getRepositoryPlaceholderKey(true, true)).toBe("kanban:loadingRepositories");
    expect(getRepositoryPlaceholderKey(true, false)).toBe("kanban:loadingRepositories");
  });

  it("reports an empty list once loading has finished", () => {
    expect(getRepositoryPlaceholderKey(false, true)).toBe("kanban:noRepositories");
  });

  it("prompts for a selection when repositories are ready", () => {
    expect(getRepositoryPlaceholderKey(false, false)).toBe("kanban:selectRepository");
  });

  it("returns keys that resolve in the English catalog", () => {
    const cases: Array<[boolean, boolean, string]> = [
      [true, true, "Loading repositories..."],
      [false, true, "No repositories"],
      [false, false, "Select repository"],
    ];
    for (const [loading, empty, expected] of cases) {
      expect(t(getRepositoryPlaceholderKey(loading, empty))).toBe(expected);
    }
  });
});
