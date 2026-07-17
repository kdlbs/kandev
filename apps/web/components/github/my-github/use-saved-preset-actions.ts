"use client";

import type { PresetOption } from "./search-bar";
import type { SidebarSelection } from "./presets-sidebar";
import { useSavedPresets } from "./use-saved-presets";

type SavedPresetActionsOptions = {
  workspaceId: string | null;
  selection: SidebarSelection;
  customQuery: string;
  resolvedPrPresets: PresetOption[];
  resolvedIssuePresets: PresetOption[];
  setProgrammaticSelection: (selection: SidebarSelection) => void;
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
};

function firstPresetSelection(
  kind: SidebarSelection["kind"],
  pr: PresetOption[],
  issue: PresetOption[],
) {
  const preset = (kind === "pr" ? pr : issue)[0];
  return {
    selection: { kind, source: "preset", id: preset?.value ?? "" } as SidebarSelection,
    filter: preset?.filter ?? "",
  };
}

export function useSavedPresetActions({
  workspaceId,
  selection,
  customQuery,
  resolvedPrPresets,
  resolvedIssuePresets,
  setProgrammaticSelection,
  setQueryImmediate,
  setRepoFilter,
}: SavedPresetActionsOptions) {
  const {
    presets: savedPresets,
    save: saveSavedPreset,
    remove: removeSavedPreset,
  } = useSavedPresets(workspaceId);

  const onConfirmSave = (label: string, defaultRepoFilter: string) => {
    const created = saveSavedPreset({
      kind: selection.kind,
      label,
      customQuery,
      repoFilter: defaultRepoFilter,
    });
    if (!created) return;
    setProgrammaticSelection({ kind: selection.kind, source: "saved", id: created.id });
    setRepoFilter(defaultRepoFilter);
  };

  const onDeleteSaved = (id: string) => {
    removeSavedPreset(id);
    if (selection.source !== "saved" || selection.id !== id) return;
    const fallback = firstPresetSelection(selection.kind, resolvedPrPresets, resolvedIssuePresets);
    setProgrammaticSelection(fallback.selection);
    setQueryImmediate(fallback.filter);
    setRepoFilter("");
  };

  return { savedPresets, onConfirmSave, onDeleteSaved };
}
