import { describe, it, expect, beforeEach, vi } from "vitest";
import { readStorage, type SavedPreset } from "./use-saved-presets";

const STORAGE_KEY = "kandev:github-presets:v1";

// Provide a simple in-memory localStorage mock so the tests are not sensitive
// to how the test runner exposes window.localStorage (e.g. Node's
// --localstorage-file flag without a valid path).
function makeLocalStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
    clear: () => store.clear(),
    get length() {
      return store.size;
    },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
}

const localStorageMock = makeLocalStorageMock();
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
    localStorageMock.clear();
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
