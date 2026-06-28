import { describe, expect, it, vi, beforeEach } from "vitest";
import { autoSelectBranch } from "./task-create-dialog-helpers";

beforeEach(() => {
  localStorage.clear();
});

describe("autoSelectBranch", () => {
  const branches = [
    { name: "main", type: "local" as const },
    { name: "feature", type: "local" as const },
  ];

  it("uses store-backed last-used branch when localStorage is not primed", () => {
    const setBranch = vi.fn();

    autoSelectBranch(branches, setBranch, {
      lastUsedBranch: "feature",
      userSettingsLoaded: false,
    });

    expect(setBranch).toHaveBeenCalledWith("feature");
  });

  it("defers preferred fallback while user settings are still loading", () => {
    const setBranch = vi.fn();

    autoSelectBranch(branches, setBranch, { userSettingsLoaded: false });

    expect(setBranch).not.toHaveBeenCalled();
  });
});
