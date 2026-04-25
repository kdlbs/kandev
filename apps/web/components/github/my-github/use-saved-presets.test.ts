import { describe, it, expect, beforeEach, vi } from "vitest";
import { readStorage, type SavedPreset } from "./use-saved-presets";

const STORAGE_KEY = "kandev:github-presets:v1";

// happy-dom's localStorage Proxy does not expose methods across VM contexts;
// stub globalThis.localStorage with a minimal in-memory implementation.
const store: Record<string, string> = {};
const localStorageMock = {
  getItem: (key: string) => store[key] ?? null,
  setItem: (key: string, value: string) => {
    store[key] = value;
  },
  removeItem: (key: string) => {
    delete store[key];
  },
  clear: () => {
    for (const key of Object.keys(store)) delete store[key];
  },
};
vi.stubGlobal("localStorage", localStorageMock);

function set(raw: string | null) {
  if (raw === null) localStorageMock.removeItem(STORAGE_KEY);
  else localStorageMock.setItem(STORAGE_KEY, raw);
}

const valid: SavedPreset = {
  id: "p_1",
  kind: "pr",
  label: "My PRs",
  customQuery: "author:@me",
  repoFilter: "",
  createdAt: "2026-01-01T00:00:00Z",
};

describe("readStorage", () => {
  beforeEach(() => {
    localStorageMock.removeItem(STORAGE_KEY);
  });

  it("returns empty array when no value is stored", () => {
    expect(readStorage()).toEqual([]);
  });

  it("returns empty array for malformed JSON", () => {
    set("not-json{");
    expect(readStorage()).toEqual([]);
  });

  it("returns empty array when parsed value is not an array", () => {
    set(JSON.stringify({ id: "p_1" }));
    expect(readStorage()).toEqual([]);
  });

  it("keeps valid entries", () => {
    set(JSON.stringify([valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops entries missing an id", () => {
    const missingId = { ...valid } as Partial<SavedPreset>;
    delete missingId.id;
    set(JSON.stringify([missingId, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops entries with invalid kind", () => {
    set(JSON.stringify([{ ...valid, kind: "commit" }, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops non-object entries", () => {
    set(JSON.stringify(["string", 42, null, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("drops entries with non-string label", () => {
    set(JSON.stringify([{ ...valid, label: 123 }, valid]));
    expect(readStorage()).toEqual([valid]);
  });

  it("accepts issue kind", () => {
    const issue: SavedPreset = { ...valid, kind: "issue" };
    set(JSON.stringify([issue]));
    expect(readStorage()).toEqual([issue]);
  });
});
