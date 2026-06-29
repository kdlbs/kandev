import { describe, expect, it, vi, beforeEach } from "vitest";
import { autoSelectBranch } from "./task-create-dialog-helpers";
import { STORAGE_KEYS } from "@/lib/settings/constants";

beforeEach(() => {
  localStorage.clear();
});

describe("autoSelectBranch", () => {
  const branches = [
    { name: "main", type: "local" as const },
    { name: "feature", type: "local" as const },
  ];

  it("prefers a store-backed branch over a divergent localStorage branch", () => {
    const setBranch = vi.fn();
    localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("feature"));

    autoSelectBranch(branches, setBranch, { lastUsedBranch: "main" });

    expect(setBranch).toHaveBeenCalledWith("main");
  });

  it("uses store-backed last-used branch when localStorage is not primed", () => {
    const setBranch = vi.fn();

    autoSelectBranch(branches, setBranch, {
      lastUsedBranch: "feature",
      userSettingsLoaded: false,
    });

    expect(setBranch).toHaveBeenCalledWith("feature");
  });

  it("uses store-backed last-used branch when localStorage branch is stale", () => {
    const setBranch = vi.fn();
    localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("deleted"));

    autoSelectBranch(branches, setBranch, {
      lastUsedBranch: "feature",
      userSettingsLoaded: true,
    });

    expect(setBranch).toHaveBeenCalledWith("feature");
  });

  it("defers preferred fallback while user settings are still loading", () => {
    const setBranch = vi.fn();

    autoSelectBranch(branches, setBranch, { userSettingsLoaded: false });

    expect(setBranch).not.toHaveBeenCalled();
  });

  it("defers when localStorage branch is stale and user settings are still loading", () => {
    const setBranch = vi.fn();
    localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("deleted"));

    autoSelectBranch(branches, setBranch, { userSettingsLoaded: false });

    expect(setBranch).not.toHaveBeenCalled();
  });

  it("defers when localStorage branch is valid but user settings are still loading", () => {
    const setBranch = vi.fn();
    localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("feature"));

    autoSelectBranch(branches, setBranch, { userSettingsLoaded: false });

    expect(setBranch).not.toHaveBeenCalled();
  });

  it("falls back to preferred branch after user settings have loaded without a valid last-used branch", () => {
    const setBranch = vi.fn();

    autoSelectBranch(branches, setBranch, { userSettingsLoaded: true });

    expect(setBranch).toHaveBeenCalledWith("main");
  });

  it("matches remote branch display names for store-backed last-used branch", () => {
    const setBranch = vi.fn();

    autoSelectBranch([{ name: "feature", type: "remote" as const, remote: "origin" }], setBranch, {
      lastUsedBranch: "origin/feature",
      userSettingsLoaded: true,
    });

    expect(setBranch).toHaveBeenCalledWith("origin/feature");
  });

  it("does not pick a branch from an empty branch list", () => {
    const setBranch = vi.fn();

    autoSelectBranch([], setBranch, { lastUsedBranch: "main", userSettingsLoaded: true });

    expect(setBranch).not.toHaveBeenCalled();
  });
});
