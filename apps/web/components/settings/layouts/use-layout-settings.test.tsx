import { type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { getBuiltInLayoutProfile } from "@/lib/layout/layout-profiles";
import { defaultState } from "@/lib/state/default-state";
import type { SavedLayout, UserSettingsResponse } from "@/lib/types/http";
import { workspaceId } from "@/lib/types/ids";

const updateUserSettings = vi.fn();
const PROFILE_ID = "profile-one";
const SECOND_PROFILE_ID = "profile-two";
const ORIGINAL_NAME = "Original";
const DRAFT_NAME = "Draft name";
const LEGACY_LAYOUT = { columns: "old-format" };

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { useLayoutSettings } from "./use-layout-settings";

function profile(overrides: Partial<SavedLayout> = {}): SavedLayout {
  return {
    id: PROFILE_ID,
    name: ORIGINAL_NAME,
    is_default: false,
    layout: getBuiltInLayoutProfile("default").layout as unknown as Record<string, unknown>,
    created_at: "2026-07-19T10:00:00.000Z",
    ...overrides,
  };
}

function responseWith(savedLayouts: SavedLayout[]): UserSettingsResponse {
  return {
    settings: {
      user_id: "user-one",
      workspace_id: workspaceId("workspace-one"),
      repository_ids: [],
      saved_layouts: savedLayouts,
      updated_at: "2026-07-19T12:00:00.000Z",
    },
  };
}

function renderLayoutSettings(savedLayouts: SavedLayout[] = []) {
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          userSettings: {
            ...defaultState.userSettings,
            savedLayouts,
            loaded: true,
          },
        }}
      >
        {children}
      </StateProvider>
    );
  }

  return renderHook(() => useLayoutSettings(), { wrapper: Wrapper });
}

beforeEach(() => {
  updateUserSettings.mockReset();
});

afterEach(cleanup);

describe("useLayoutSettings selection and drafts", () => {
  it("selects the effective default on initialization", () => {
    const customDefault = profile({ is_default: true });
    const builtIn = renderLayoutSettings();
    const custom = renderLayoutSettings([customDefault]);

    expect(builtIn.result.current).toMatchObject({
      selection: { kind: "built-in", id: "default" },
      selectedIsDefault: true,
      defaultActionDisabled: true,
      defaultActionLabel: "Default",
      isDirty: false,
    });
    expect(custom.result.current).toMatchObject({
      selection: { kind: "custom", id: PROFILE_ID },
      selectedCustom: customDefault,
      selectedIsDefault: true,
      defaultActionDisabled: false,
      defaultActionLabel: "Use built-in Default",
      isDirty: false,
    });
  });

  it("duplicates and renames a profile, then cancel restores the baseline", () => {
    const { result } = renderLayoutSettings();

    act(() => result.current.duplicate());
    const duplicateId = result.current.selection.id;
    expect(result.current).toMatchObject({
      selection: { kind: "custom", id: duplicateId },
      selectedName: "Default copy",
      isDirty: true,
    });

    act(() => result.current.updateSelected({ name: DRAFT_NAME }));
    expect(result.current.selectedName).toBe(DRAFT_NAME);

    act(() => result.current.cancel());
    expect(result.current).toMatchObject({
      selection: { kind: "built-in", id: "default" },
      profiles: [],
      isDirty: false,
      editorReset: 1,
    });
  });

  it("creates an editable profile from the built-in Default", () => {
    const { result } = renderLayoutSettings([profile()]);

    act(() => result.current.create());

    expect(result.current.profiles).toHaveLength(2);
    expect(result.current.selectedCustom).toMatchObject({
      id: result.current.selection.id,
      name: "Untitled layout",
      is_default: false,
    });
    expect(result.current.compatibility?.status).toBe("editable");
    expect(result.current.isDirty).toBe(true);
  });

  it("sets a custom default and clears it through the built-in Default", () => {
    const second = profile({ id: SECOND_PROFILE_ID, name: "Second" });
    const { result } = renderLayoutSettings([profile(), second]);

    act(() => result.current.setSelection({ kind: "custom", id: SECOND_PROFILE_ID }));
    act(() => result.current.setDefault());
    expect(result.current.profiles.map((candidate) => candidate.is_default)).toEqual([false, true]);

    act(() => result.current.setSelection({ kind: "built-in", id: "default" }));
    expect(result.current.defaultActionDisabled).toBe(false);
    act(() => result.current.setDefault());

    expect(result.current.profiles.map((candidate) => candidate.is_default)).toEqual([
      false,
      false,
    ]);
    expect(result.current.selectedIsDefault).toBe(true);
    expect(result.current.defaultActionDisabled).toBe(true);
  });

  it("disables an invalid legacy non-default but permits clearing an invalid default", () => {
    const unavailable = profile({ layout: LEGACY_LAYOUT });
    const nonDefault = renderLayoutSettings([unavailable]);

    act(() => nonDefault.result.current.setSelection({ kind: "custom", id: PROFILE_ID }));
    expect(nonDefault.result.current).toMatchObject({
      compatibility: { status: "legacy" },
      defaultActionDisabled: true,
      selectedIsDefault: false,
    });

    const invalidDefault = renderLayoutSettings([
      profile({ layout: LEGACY_LAYOUT, is_default: true }),
    ]);
    expect(invalidDefault.result.current).toMatchObject({
      selection: { kind: "built-in", id: "default" },
      selectedIsDefault: true,
      defaultActionDisabled: true,
    });

    act(() => invalidDefault.result.current.setSelection({ kind: "custom", id: PROFILE_ID }));
    expect(invalidDefault.result.current).toMatchObject({
      compatibility: { status: "legacy" },
      defaultActionDisabled: false,
      defaultActionLabel: "Use built-in Default",
      selectedIsDefault: false,
    });

    act(() => invalidDefault.result.current.setDefault());
    expect(invalidDefault.result.current.profiles[0].is_default).toBe(false);
    expect(invalidDefault.result.current.defaultActionDisabled).toBe(true);
  });
});

describe("useLayoutSettings built-in editing", () => {
  it("edits the built-in Default directly as a custom default draft", () => {
    const { result } = renderLayoutSettings();
    const layout = getBuiltInLayoutProfile("default").layout;
    layout.columns = layout.columns
      .map((column) => ({
        ...column,
        groups: column.groups
          .map((group) => ({
            ...group,
            panels: group.panels.filter((panel) => panel.id !== "terminal-default"),
          }))
          .filter((group) => group.panels.length > 0),
      }))
      .filter((column) => column.groups.length > 0);

    act(() => result.current.updateLayout(layout));

    expect(result.current).toMatchObject({
      selection: { kind: "custom", id: result.current.selection.id },
      selectedName: "Default custom",
      selectedIsDefault: true,
      isDirty: true,
    });
    expect(result.current.selectedCustom?.is_default).toBe(true);
    expect(
      result.current.editorLayout?.columns
        .flatMap((column) => column.groups)
        .flatMap((group) => group.panels)
        .map((panel) => panel.id),
    ).not.toContain("terminal-default");
  });
});

describe("useLayoutSettings persistence", () => {
  it("adopts the successful PATCH response as the authoritative baseline", async () => {
    const authoritative = profile({ name: "Server name", is_default: true });
    updateUserSettings.mockResolvedValueOnce(responseWith([authoritative]));
    const { result } = renderLayoutSettings([profile()]);

    act(() => result.current.setSelection({ kind: "custom", id: PROFILE_ID }));
    act(() => result.current.updateSelected({ name: DRAFT_NAME }));
    await act(async () => result.current.save());

    expect(updateUserSettings).toHaveBeenCalledWith({
      saved_layouts: [profile({ name: DRAFT_NAME })],
    });
    expect(result.current).toMatchObject({
      profiles: [authoritative],
      baseline: [authoritative],
      selection: { kind: "custom", id: PROFILE_ID },
      selectedName: "Server name",
      saveStatus: "success",
      error: null,
      isDirty: false,
    });
  });

  it("preserves the unsaved draft and reports a failed PATCH", async () => {
    updateUserSettings.mockRejectedValueOnce(new Error("save failed"));
    const original = profile();
    const { result } = renderLayoutSettings([original]);

    act(() => result.current.setSelection({ kind: "custom", id: PROFILE_ID }));
    act(() => result.current.updateSelected({ name: DRAFT_NAME }));
    await act(async () => result.current.save());

    expect(result.current).toMatchObject({
      profiles: [profile({ name: DRAFT_NAME })],
      baseline: [original],
      selectedName: DRAFT_NAME,
      saveStatus: "error",
      error: "save failed",
      isDirty: true,
    });
  });
});
