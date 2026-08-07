import { describe, expect, it } from "vitest";
import * as model from "./saved-preset-model";
import type { SavedPreset, SavedPresetKind } from "./saved-preset-model";

type DefaultTarget = {
  selection: { kind: SavedPresetKind; source: "preset" | "saved"; id: string };
  query: string;
  repoFilter: string;
};

type FutureModel = {
  setSavedPresetDefault: (
    presets: SavedPreset[],
    kind: SavedPresetKind,
    id: string | null,
  ) => SavedPreset[];
  findDefaultSavedPreset: (
    presets: SavedPreset[],
    kind: SavedPresetKind,
  ) => SavedPreset | undefined;
  resolveDefaultSidebarTarget: (
    kind: SavedPresetKind,
    presets: SavedPreset[],
    configured: Array<{ value: string; filter: string }>,
  ) => DefaultTarget;
};

function futureFunction<K extends keyof FutureModel>(name: K): FutureModel[K] {
  const fn = (model as unknown as Partial<FutureModel>)[name];
  expect(fn, `${name} export`).toBeTypeOf("function");
  return fn as FutureModel[K];
}

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
  it.each([
    { field: "customQuery", value: undefined },
    { field: "customQuery", value: 42 },
    { field: "createdAt", value: undefined },
    { field: "createdAt", value: 42 },
  ])("rejects an entry with an invalid $field", ({ field, value }) => {
    const valid = preset("valid", "pr");
    const malformed = { ...preset("malformed", "pr"), [field]: value };

    expect(model.readSavedPresets([malformed, valid])).toEqual([valid]);
  });

  it("returns only validated saved-preset fields", () => {
    const valid = preset("valid", "pr");

    expect(model.readSavedPresets([{ ...valid, serverOnly: "discard me" }])).toEqual([valid]);
  });
});

describe("saved preset defaults", () => {
  it("replaces one kind's default without changing the other kind", () => {
    const setDefault = futureFunction("setSavedPresetDefault");
    const initial = [
      preset("pr-a", "pr", true),
      preset("pr-b", "pr"),
      preset("issue-a", "issue", true),
    ];

    const next = setDefault(initial, "pr", "pr-b");

    expect(next.map(({ id, isDefault }) => ({ id, isDefault }))).toEqual([
      { id: "pr-a", isDefault: false },
      { id: "pr-b", isDefault: true },
      { id: "issue-a", isDefault: true },
    ]);
    expect(initial[0].isDefault).toBe(true);
  });

  it("clears only the requested kind's default", () => {
    const setDefault = futureFunction("setSavedPresetDefault");
    const initial = [preset("pr-a", "pr", true), preset("issue-a", "issue", true)];

    const next = setDefault(initial, "pr", null);

    expect(next.map(({ id, isDefault }) => ({ id, isDefault }))).toEqual([
      { id: "pr-a", isDefault: false },
      { id: "issue-a", isDefault: true },
    ]);
  });

  it("keeps the current default when the requested saved query does not exist", () => {
    const setDefault = futureFunction("setSavedPresetDefault");
    const initial = [preset("pr-a", "pr", true)];

    expect(setDefault(initial, "pr", "missing")).toBe(initial);
  });

  it("finds the effective default for a result kind", () => {
    const findDefault = futureFunction("findDefaultSavedPreset");
    const issueDefault = preset("issue-a", "issue", true);

    expect(findDefault([preset("pr-a", "pr", true), issueDefault], "issue")).toBe(issueDefault);
  });

  it("resolves a saved default with its query and repository", () => {
    const resolveTarget = futureFunction("resolveDefaultSidebarTarget");
    const saved = preset("pr-default", "pr", true);

    expect(resolveTarget("pr", [saved], [{ value: "review", filter: "review:@me" }])).toEqual({
      selection: { kind: "pr", source: "saved", id: "pr-default" },
      query: "query:pr-default",
      repoFilter: "repo/pr-default",
    });
  });

  it("falls back to the first configured query with all repositories", () => {
    const resolveTarget = futureFunction("resolveDefaultSidebarTarget");

    expect(resolveTarget("issue", [], [{ value: "assigned", filter: "assignee:@me" }])).toEqual({
      selection: { kind: "issue", source: "preset", id: "assigned" },
      query: "assignee:@me",
      repoFilter: "",
    });
  });
});
