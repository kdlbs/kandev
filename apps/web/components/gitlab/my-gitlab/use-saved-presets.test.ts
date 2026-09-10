import { describe, it, expect, beforeEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { __resetSnapshotForTests, useSavedPresets, type SavedPreset } from "./use-saved-presets";

const STORAGE_KEY = "kandev:gitlab-presets:v1";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));

function set(raw: string | null) {
  if (raw === null) window.localStorage.removeItem(STORAGE_KEY);
  else window.localStorage.setItem(STORAGE_KEY, raw);
}

const valid: SavedPreset = {
  id: "g_1",
  kind: "mr",
  label: "My MRs",
  customQuery: "scope=created_by_me",
  projectFilter: "",
  milestone: "",
  preset: "",
  createdAt: "2026-01-01T00:00:00Z",
};

describe("useSavedPresets (gitlab)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    __resetSnapshotForTests();
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { gitlab_saved_presets: [] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);
  });

  it("save appends a preset and assigns id + createdAt", () => {
    const { result } = renderHook(() => useSavedPresets());
    act(() => {
      result.current.save({
        kind: "mr",
        label: "My MRs",
        customQuery: "scope=created_by_me",
        projectFilter: "",
        milestone: "",
        preset: "",
      });
    });
    expect(result.current.presets).toHaveLength(1);
    const [p] = result.current.presets;
    expect(p.label).toBe("My MRs");
    expect(p.kind).toBe("mr");
    expect(p.id).toMatch(/^g_/);
    expect(p.createdAt).toBeTruthy();
  });

  it("remove deletes the preset", () => {
    const { result } = renderHook(() => useSavedPresets());
    let created!: SavedPreset;
    act(() => {
      created = result.current.save({
        kind: "issue",
        label: "Triage",
        customQuery: "labels=bug",
        projectFilter: "",
        milestone: "",
        preset: "",
      });
    });
    expect(result.current.presets).toHaveLength(1);
    act(() => result.current.remove(created.id));
    expect(result.current.presets).toHaveLength(0);
  });

  it("two hook instances stay in sync via the module-level store", () => {
    const a = renderHook(() => useSavedPresets());
    const b = renderHook(() => useSavedPresets());
    act(() => {
      a.result.current.save({
        kind: "mr",
        label: "Shared",
        customQuery: "q",
        projectFilter: "",
        milestone: "",
        preset: "",
      });
    });
    expect(a.result.current.presets).toHaveLength(1);
    expect(b.result.current.presets).toHaveLength(1);
    expect(b.result.current.presets[0].label).toBe("Shared");
  });

  it("ignores stale local presets when backend settings are empty", async () => {
    set(JSON.stringify([valid]));
    __resetSnapshotForTests();

    const { result } = renderHook(() => useSavedPresets());

    await waitFor(() => expect(result.current.presets).toEqual([]));
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});

const LEGACY_CREATED_AT = "2025-01-01T00:00:00Z";

describe("useSavedPresets (gitlab) — legacy preset field normalization", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    __resetSnapshotForTests();
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);
  });

  it("keeps a legacy preset with neither a milestone nor a preset key, defaulting both to empty", async () => {
    const legacy = {
      id: "g_legacy",
      kind: "issue",
      label: "Legacy triage",
      customQuery: "labels=bug",
      projectFilter: "",
      createdAt: LEGACY_CREATED_AT,
    };
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { gitlab_saved_presets: [legacy] },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    const { result } = renderHook(() => useSavedPresets());

    await waitFor(() => expect(result.current.presets).toHaveLength(1));
    expect(result.current.presets[0]).toMatchObject({
      id: "g_legacy",
      milestone: "",
      preset: "",
    });
  });

  it("normalizes a non-string milestone/preset (number, null, object) to empty rather than dropping the entry", async () => {
    const withBadTypes = [
      {
        id: "g_number",
        kind: "issue",
        label: "Number",
        customQuery: "",
        projectFilter: "",
        createdAt: LEGACY_CREATED_AT,
        milestone: 42,
        preset: null,
      },
      {
        id: "g_object",
        kind: "issue",
        label: "Object",
        customQuery: "",
        projectFilter: "",
        createdAt: LEGACY_CREATED_AT,
        milestone: { title: "Next" },
        preset: 7,
      },
    ];
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { gitlab_saved_presets: withBadTypes },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    const { result } = renderHook(() => useSavedPresets());

    await waitFor(() => expect(result.current.presets).toHaveLength(2));
    expect(result.current.presets[0]).toMatchObject({ id: "g_number", milestone: "", preset: "" });
    expect(result.current.presets[1]).toMatchObject({ id: "g_object", milestone: "", preset: "" });
  });

  it("still rejects an entry missing a required legacy field", async () => {
    const missingLabel = [
      {
        id: "g_bad",
        kind: "issue",
        customQuery: "",
        projectFilter: "",
        createdAt: LEGACY_CREATED_AT,
      },
    ];
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { gitlab_saved_presets: missingLabel },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);

    const { result } = renderHook(() => useSavedPresets());

    await waitFor(() => expect(fetchUserSettings).toHaveBeenCalled());
    expect(result.current.presets).toEqual([]);
  });
});
