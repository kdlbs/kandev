"use client";

import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from "react";
import { useAppStore } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import {
  createLayoutProfileId,
  deleteLayoutProfile,
  duplicateLayoutProfile,
  getBuiltInLayoutOverride,
  getBuiltInLayoutOverrideSourceId,
  getBuiltInLayoutProfile,
  getLayoutProfileCompatibility,
  resetBuiltInLayoutOverride,
  resolveEffectiveDefaultLayout,
  setDefaultLayoutProfile,
  upsertBuiltInLayoutOverride,
  validateReusableLayout,
} from "@/lib/layout/layout-profiles";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { LayoutState } from "@/lib/state/layout-manager";
import type { SavedLayout } from "@/lib/types/http";
import type { LayoutProfileSelection } from "./layout-profile-list";

export type SaveStatus = "idle" | "loading" | "success" | "error";

function defaultSelection(profiles: SavedLayout[]): LayoutProfileSelection {
  const effective = resolveEffectiveDefaultLayout(profiles);
  if (effective.source === "built-in") return { kind: "built-in", id: "default" };
  const builtInId = getBuiltInLayoutOverrideSourceId(effective.profile);
  return builtInId
    ? { kind: "built-in", id: builtInId }
    : { kind: "custom", id: effective.profile.id };
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Failed to save layout profiles";
}

function getDefaultActionLabel(isCustomDefault: boolean, selectedIsDefault: boolean) {
  if (isCustomDefault) return "Use built-in Default";
  if (selectedIsDefault) return "Default";
  return "Use as default";
}

function attempt(setError: Dispatch<SetStateAction<string | null>>, action: () => void) {
  try {
    action();
  } catch (error) {
    setError(errorMessage(error));
  }
}

function useLayoutProfileDrafts(savedLayouts: SavedLayout[]) {
  const [baseline, setBaseline] = useState(() => structuredClone(savedLayouts));
  const [profiles, setProfiles] = useState(() => structuredClone(savedLayouts));
  const [selection, setSelection] = useState<LayoutProfileSelection>(() =>
    defaultSelection(savedLayouts),
  );
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [editorReset, setEditorReset] = useState(0);
  const baselineKey = useMemo(() => JSON.stringify(baseline), [baseline]);
  const profilesKey = useMemo(() => JSON.stringify(profiles), [profiles]);
  const storeKey = useMemo(() => JSON.stringify(savedLayouts), [savedLayouts]);
  const isDirty = baselineKey !== profilesKey;

  useEffect(() => {
    if (storeKey === baselineKey || isDirty) return;
    const next = structuredClone(savedLayouts);
    setBaseline(next);
    setProfiles(structuredClone(next));
    setSelection(defaultSelection(next));
  }, [baselineKey, isDirty, savedLayouts, storeKey]);

  const replaceProfiles = (next: SavedLayout[]) => {
    setProfiles(next);
    setSaveStatus("idle");
    setError(null);
  };
  const cancel = () => {
    setProfiles(structuredClone(baseline));
    setSelection(defaultSelection(baseline));
    setSaveStatus("idle");
    setError(null);
    setEditorReset((value) => value + 1);
  };
  return {
    baseline,
    profiles,
    selection,
    saveStatus,
    error,
    editorReset,
    isDirty,
    profilesKey,
    setBaseline,
    setProfiles,
    setSelection,
    setSaveStatus,
    setError,
    setEditorReset,
    replaceProfiles,
    cancel,
  };
}

type Drafts = ReturnType<typeof useLayoutProfileDrafts>;

function selectedState(drafts: Drafts) {
  const selectedCustom =
    drafts.selection.kind === "custom"
      ? (drafts.profiles.find((profile) => profile.id === drafts.selection.id) ?? null)
      : null;
  const selectedBuiltIn =
    drafts.selection.kind === "built-in" ? getBuiltInLayoutProfile(drafts.selection.id) : null;
  const selectedBuiltInOverride = selectedBuiltIn
    ? getBuiltInLayoutOverride(drafts.profiles, selectedBuiltIn.id)
    : null;
  const editableProfile = selectedCustom ?? selectedBuiltInOverride;
  const compatibility = editableProfile ? getLayoutProfileCompatibility(editableProfile) : null;
  const editorLayout =
    compatibility?.status === "editable" ? compatibility.layout : (selectedBuiltIn?.layout ?? null);
  return {
    selectedCustom,
    selectedBuiltIn,
    selectedBuiltInOverride,
    compatibility,
    editorLayout,
  };
}

type SelectedState = ReturnType<typeof selectedState>;

function updateBuiltInDefault(drafts: Drafts, selected: SelectedState) {
  if (!selected.selectedBuiltIn) return false;
  if (selected.selectedBuiltIn.id === "default" || selected.selectedBuiltInOverride?.is_default) {
    drafts.replaceProfiles(setDefaultLayoutProfile(drafts.profiles, null));
    return true;
  }
  drafts.replaceProfiles(
    upsertBuiltInLayoutOverride(
      drafts.profiles,
      selected.selectedBuiltIn.id,
      selected.editorLayout ?? selected.selectedBuiltIn.layout,
      { createdAt: new Date().toISOString(), isDefault: true },
    ),
  );
  return true;
}

function useProfileActions(drafts: Drafts, selected: SelectedState) {
  const selectedName = selected.selectedBuiltIn?.name ?? selected.selectedCustom?.name ?? "Layout";
  const duplicate = () =>
    attempt(drafts.setError, () => {
      const id = createLayoutProfileId();
      const source =
        selected.selectedBuiltInOverride ?? selected.selectedBuiltIn ?? drafts.selection.id;
      drafts.replaceProfiles(
        duplicateLayoutProfile(drafts.profiles, source, {
          id,
          name: `${selectedName} copy`,
          createdAt: new Date().toISOString(),
        }),
      );
      drafts.setSelection({ kind: "custom", id });
    });
  const create = () =>
    attempt(drafts.setError, () => {
      const id = createLayoutProfileId();
      drafts.replaceProfiles(
        duplicateLayoutProfile(drafts.profiles, getBuiltInLayoutProfile("default"), {
          id,
          name: "Untitled layout",
          createdAt: new Date().toISOString(),
        }),
      );
      drafts.setSelection({ kind: "custom", id });
    });
  const updateSelected = (updates: Partial<SavedLayout>) => {
    if (!selected.selectedCustom) return;
    drafts.replaceProfiles(
      drafts.profiles.map((profile) =>
        profile.id === selected.selectedCustom?.id ? { ...profile, ...updates } : profile,
      ),
    );
  };
  const updateLayout = (layout: LayoutState) => {
    const validation = validateReusableLayout(layout);
    if (!validation.valid) {
      drafts.setError(validation.issues.map((issue) => issue.message).join(". "));
      return;
    }
    const nextLayout = validation.layout as unknown as Record<string, unknown>;
    if (selected.selectedCustom) {
      updateSelected({ layout: nextLayout });
      return;
    }
    const builtIn = selected.selectedBuiltIn;
    if (!builtIn) return;
    attempt(drafts.setError, () => {
      const isDefault =
        builtIn.id === "default"
          ? selected.selectedBuiltInOverride?.is_default === true ||
            !drafts.profiles.some((profile) => profile.is_default)
          : undefined;
      drafts.replaceProfiles(
        upsertBuiltInLayoutOverride(drafts.profiles, builtIn.id, nextLayout, {
          createdAt: new Date().toISOString(),
          isDefault,
        }),
      );
    });
  };
  const setDefault = () =>
    attempt(drafts.setError, () => {
      if (updateBuiltInDefault(drafts, selected)) return;
      const profileId =
        drafts.selection.kind === "custom" && !selected.selectedCustom?.is_default
          ? drafts.selection.id
          : null;
      drafts.replaceProfiles(setDefaultLayoutProfile(drafts.profiles, profileId));
    });
  const deleteSelected = () =>
    attempt(drafts.setError, () => {
      if (!selected.selectedCustom) return;
      drafts.replaceProfiles(deleteLayoutProfile(drafts.profiles, selected.selectedCustom.id));
      drafts.setSelection({ kind: "built-in", id: "default" });
    });
  const resetBuiltIn = () => {
    if (!selected.selectedBuiltInOverride || !selected.selectedBuiltIn) return;
    drafts.replaceProfiles(
      resetBuiltInLayoutOverride(drafts.profiles, selected.selectedBuiltIn.id),
    );
    drafts.setEditorReset((value) => value + 1);
  };
  return {
    selectedName,
    duplicate,
    create,
    updateSelected,
    updateLayout,
    setDefault,
    deleteSelected,
    resetBuiltIn,
  };
}

function useSaveProfiles(drafts: Drafts) {
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  return async () => {
    if (!drafts.isDirty || drafts.saveStatus === "loading") return;
    if (drafts.profiles.some((profile) => !profile.name.trim())) {
      drafts.setError("Layout profile names must not be empty");
      drafts.setSaveStatus("error");
      return;
    }
    drafts.setSaveStatus("loading");
    drafts.setError(null);
    try {
      const response = await updateUserSettings({ saved_layouts: drafts.profiles });
      const authoritative = mapUserSettingsResponse(response);
      const next = structuredClone(authoritative.savedLayouts);
      setUserSettings(authoritative);
      drafts.setBaseline(next);
      drafts.setProfiles(structuredClone(next));
      drafts.setSelection((current) =>
        current.kind === "built-in" || next.some((profile) => profile.id === current.id)
          ? current
          : defaultSelection(next),
      );
      drafts.setSaveStatus("success");
    } catch (error) {
      drafts.setSaveStatus("error");
      drafts.setError(errorMessage(error));
      throw error;
    }
  };
}

function isSelectedEffectiveDefault(
  selection: LayoutProfileSelection,
  effectiveDefault: ReturnType<typeof resolveEffectiveDefaultLayout>,
) {
  if (effectiveDefault.source === "built-in") {
    return selection.kind === "built-in" && selection.id === "default";
  }
  if (selection.kind === "custom") return effectiveDefault.profile.id === selection.id;
  return getBuiltInLayoutOverrideSourceId(effectiveDefault.profile) === selection.id;
}

function getDefaultActionState(drafts: Drafts, selected: SelectedState) {
  const effectiveDefault = resolveEffectiveDefaultLayout(drafts.profiles);
  const selectedIsDefault = isSelectedEffectiveDefault(drafts.selection, effectiveDefault);
  const selectedSavedDefault = Boolean(
    selected.selectedCustom?.is_default || selected.selectedBuiltInOverride?.is_default,
  );
  const disabled =
    (drafts.selection.kind === "built-in" && selectedIsDefault) ||
    (selected.compatibility?.status === "legacy" && !selectedSavedDefault);
  const builtInDefaultSelected =
    drafts.selection.kind === "built-in" && drafts.selection.id === "default";
  const label =
    builtInDefaultSelected && selectedIsDefault
      ? "Default"
      : getDefaultActionLabel(selectedSavedDefault, selectedIsDefault);
  return { disabled, label, selectedIsDefault };
}

export function useLayoutSettings() {
  const savedLayouts = useAppStore((state) => state.userSettings.savedLayouts);
  const drafts = useLayoutProfileDrafts(savedLayouts);
  const selected = selectedState(drafts);
  const actions = useProfileActions(drafts, selected);
  const save = useSaveProfiles(drafts);
  const defaultAction = getDefaultActionState(drafts, selected);
  return {
    ...drafts,
    ...selected,
    ...actions,
    save,
    defaultActionDisabled: defaultAction.disabled,
    defaultActionLabel: defaultAction.label,
    selectedIsDefault: defaultAction.selectedIsDefault,
  };
}
