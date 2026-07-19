"use client";

import { useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { useAppStore } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import {
  deleteLayoutProfile,
  duplicateLayoutProfile,
  getBuiltInLayoutProfile,
  getLayoutProfileCompatibility,
  resolveEffectiveDefaultLayout,
  setDefaultLayoutProfile,
  validateReusableLayout,
} from "@/lib/layout/layout-profiles";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { LayoutState } from "@/lib/state/layout-manager";
import type { SavedLayout } from "@/lib/types/http";
import type { LayoutProfileSelection } from "./layout-profile-list";

export type SaveStatus = "idle" | "loading" | "success" | "error";

function defaultSelection(profiles: SavedLayout[]): LayoutProfileSelection {
  const effective = resolveEffectiveDefaultLayout(profiles);
  return effective.source === "custom"
    ? { kind: "custom", id: effective.profile.id }
    : { kind: "built-in", id: "default" };
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Failed to save layout profiles";
}

function getDefaultActionLabel(selectedCustom: SavedLayout | null, selectedIsDefault: boolean) {
  if (selectedCustom?.is_default) return "Use built-in Default";
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
  const baselineKey = JSON.stringify(baseline);
  const isDirty = baselineKey !== JSON.stringify(profiles);
  const storeKey = JSON.stringify(savedLayouts);

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
    setBaseline,
    setProfiles,
    setSelection,
    setSaveStatus,
    setError,
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
  const compatibility = selectedCustom ? getLayoutProfileCompatibility(selectedCustom) : null;
  const editorLayout =
    selectedBuiltIn?.layout ?? (compatibility?.status === "editable" ? compatibility.layout : null);
  return { selectedCustom, selectedBuiltIn, compatibility, editorLayout };
}

function useProfileActions(drafts: Drafts, selected: ReturnType<typeof selectedState>) {
  const selectedName = selected.selectedBuiltIn?.name ?? selected.selectedCustom?.name ?? "Layout";
  const duplicate = () =>
    attempt(drafts.setError, () => {
      const id = `layout-${globalThis.crypto.randomUUID()}`;
      const source = selected.selectedBuiltIn ?? drafts.selection.id;
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
      const id = `layout-${globalThis.crypto.randomUUID()}`;
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
    updateSelected({ layout: validation.layout as unknown as Record<string, unknown> });
  };
  const setDefault = () =>
    attempt(drafts.setError, () => {
      const profileId =
        drafts.selection.kind === "custom" && !selected.selectedCustom?.is_default
          ? drafts.selection.id
          : null;
      if (drafts.selection.kind === "built-in" && drafts.selection.id !== "default") return;
      drafts.replaceProfiles(setDefaultLayoutProfile(drafts.profiles, profileId));
    });
  const deleteSelected = () =>
    attempt(drafts.setError, () => {
      if (!selected.selectedCustom) return;
      drafts.replaceProfiles(deleteLayoutProfile(drafts.profiles, selected.selectedCustom.id));
      drafts.setSelection({ kind: "built-in", id: "default" });
    });
  return {
    selectedName,
    duplicate,
    create,
    updateSelected,
    updateLayout,
    setDefault,
    deleteSelected,
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
        current.kind === "custom" && next.some((profile) => profile.id === current.id)
          ? current
          : defaultSelection(next),
      );
      drafts.setSaveStatus("success");
    } catch (error) {
      drafts.setSaveStatus("error");
      drafts.setError(errorMessage(error));
    }
  };
}

export function useLayoutSettings() {
  const savedLayouts = useAppStore((state) => state.userSettings.savedLayouts);
  const drafts = useLayoutProfileDrafts(savedLayouts);
  const selected = selectedState(drafts);
  const actions = useProfileActions(drafts, selected);
  const save = useSaveProfiles(drafts);
  const effectiveDefault = resolveEffectiveDefaultLayout(drafts.profiles);
  const defaultActionVisible =
    drafts.selection.kind === "custom" || drafts.selection.id === "default";
  const selectedIsDefault =
    (effectiveDefault.source === "custom" &&
      drafts.selection.kind === "custom" &&
      effectiveDefault.profile.id === drafts.selection.id) ||
    (effectiveDefault.source === "built-in" &&
      drafts.selection.kind === "built-in" &&
      drafts.selection.id === "default");
  const defaultActionDisabled =
    (drafts.selection.kind === "built-in" && Boolean(selectedIsDefault)) ||
    (selected.compatibility?.status === "legacy" && !selected.selectedCustom?.is_default);
  const defaultActionLabel = getDefaultActionLabel(
    selected.selectedCustom,
    Boolean(selectedIsDefault),
  );
  return {
    ...drafts,
    ...selected,
    ...actions,
    save,
    defaultActionVisible,
    defaultActionDisabled,
    defaultActionLabel,
    selectedIsDefault: Boolean(selectedIsDefault),
  };
}
