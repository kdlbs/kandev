import { describe, expect, it } from "vitest";
import {
  findDefaultSavedPreset,
  readSavedPresets,
  resolveDefaultSidebarTarget,
  setSavedPresetDefault,
  type SavedPreset,
  type SavedPresetKind,
} from "./saved-preset-model";

function preset(id: string, kind: SavedPresetKind, isDefault = false): SavedPreset {
  return {
    id,
    kind,
    label: id,
    customQuery: `query:${id}`,
    repoFilter: `repo/${id}`,
    createdAt: "2026-08-07T00:00:00Z",
    isDefault,
  };
}

describe("saved preset parsing", () => {
  it("returns an empty list for non-array input", () => {
    expect(readSavedPresets(undefined)).toEqual([]);
  });

  it.each([
    { field: "customQuery", value: undefined },
    { field: "customQuery", value: 42 },
    { field: "createdAt", value: undefined },
    { field: "createdAt", value: 42 },
  ])("rejects an entry with an invalid $field", ({ field, value }) => {
    const valid = preset("valid", "pr");
    const malformed = { ...preset("malformed", "pr"), [field]: value };

    expect(readSavedPresets([malformed, valid])).toEqual([valid]);
  });

  it("drops entries that omit required query or timestamp fields", () => {
    const valid = preset("valid", "pr");
    const missingCustomQuery = {
      id: "missing-query",
      kind: "pr",
      label: "Missing query",
      repoFilter: "repo/missing-query",
      createdAt: "2026-08-07T00:00:00Z",
      isDefault: false,
    };
    const missingCreatedAt = {
      id: "missing-created-at",
      kind: "pr",
      label: "Missing created at",
      customQuery: "query:missing-created-at",
      repoFilter: "repo/missing-created-at",
      isDefault: false,
    };

    expect(readSavedPresets([missingCustomQuery, missingCreatedAt, valid])).toEqual([valid]);
  });

  it("returns only validated saved-preset fields", () => {
    const valid = preset("valid", "pr");

    expect(readSavedPresets([{ ...valid, serverOnly: "discard me" }])).toEqual([valid]);
  });
});

describe("saved preset defaults", () => {
  it("replaces one kind's default without changing the other kind", () => {
    const initial = [
      preset("pr-a", "pr", true),
      preset("pr-b", "pr"),
      preset("issue-a", "issue", true),
    ];

    const next = setSavedPresetDefault(initial, "pr", "pr-b");

    expect(next.map(({ id, isDefault }) => ({ id, isDefault }))).toEqual([
      { id: "pr-a", isDefault: false },
      { id: "pr-b", isDefault: true },
      { id: "issue-a", isDefault: true },
    ]);
    expect(initial[0].isDefault).toBe(true);
  });

  it("clears only the requested kind's default", () => {
    const initial = [preset("pr-a", "pr", true), preset("issue-a", "issue", true)];

    const next = setSavedPresetDefault(initial, "pr", null);

    expect(next.map(({ id, isDefault }) => ({ id, isDefault }))).toEqual([
      { id: "pr-a", isDefault: false },
      { id: "issue-a", isDefault: true },
    ]);
  });

  it("keeps the current default when the requested saved query does not exist", () => {
    const initial = [preset("pr-a", "pr", true)];

    expect(setSavedPresetDefault(initial, "pr", "missing")).toBe(initial);
  });

  it("finds the effective default for a result kind", () => {
    const issueDefault = preset("issue-a", "issue", true);

    expect(findDefaultSavedPreset([preset("pr-a", "pr", true), issueDefault], "issue")).toBe(
      issueDefault,
    );
  });

  it("resolves a saved default with its query and repository", () => {
    const saved = preset("pr-default", "pr", true);

    expect(
      resolveDefaultSidebarTarget("pr", [saved], [{ value: "review", filter: "review:@me" }]),
    ).toEqual({
      selection: { kind: "pr", source: "saved", id: "pr-default" },
      query: "query:pr-default",
      repoFilter: "repo/pr-default",
    });
  });

  it("falls back to the first configured query with all repositories", () => {
    expect(
      resolveDefaultSidebarTarget("issue", [], [{ value: "assigned", filter: "assignee:@me" }]),
    ).toEqual({
      selection: { kind: "issue", source: "preset", id: "assigned" },
      query: "assignee:@me",
      repoFilter: "",
    });
  });

  it("falls back to an empty query when no query is configured", () => {
    expect(resolveDefaultSidebarTarget("issue", [], [])).toEqual({
      selection: { kind: "issue", source: "preset", id: "" },
      query: "",
      repoFilter: "",
    });
  });
});
