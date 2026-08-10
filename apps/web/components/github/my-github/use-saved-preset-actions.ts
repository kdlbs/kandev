"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import type { PresetOption } from "./search-bar";
import type { SidebarSelection } from "./presets-sidebar";
import type { SavedPreset, useSavedPresets } from "./use-saved-presets";

type SavedPresetActionsOptions = {
  selection: SidebarSelection;
  customQuery: string;
  resolvedPrPresets: PresetOption[];
  resolvedIssuePresets: PresetOption[];
  setProgrammaticSelection: (selection: SidebarSelection) => void;
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  savedPresetStore: ReturnType<typeof useSavedPresets>;
  markSearchInteracted: () => void;
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
  selection,
  customQuery,
  resolvedPrPresets: prPresets,
  resolvedIssuePresets: issuePresets,
  setProgrammaticSelection,
  setQueryImmediate,
  setRepoFilter,
  savedPresetStore,
  markSearchInteracted,
}: SavedPresetActionsOptions) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [defaultMutationPending, setDefaultMutationPending] = useState(false);
  const defaultMutationPendingRef = useRef(false);
  const {
    presets: savedPresets,
    save: saveSavedPreset,
    remove: removeSavedPreset,
    setDefault: setSavedPresetDefault,
  } = savedPresetStore;

  const onConfirmSave = useCallback(
    (label: string, defaultRepoFilter: string) => {
      markSearchInteracted();
      const created = saveSavedPreset({
        kind: selection.kind,
        label,
        customQuery,
        repoFilter: defaultRepoFilter,
      });
      if (!created) return;
      setProgrammaticSelection({ kind: selection.kind, source: "saved", id: created.id });
      setQueryImmediate(customQuery);
      setRepoFilter(defaultRepoFilter);
    },
    [
      customQuery,
      markSearchInteracted,
      saveSavedPreset,
      selection.kind,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
    ],
  );

  const onDeleteSaved = useCallback(
    (id: string) => {
      markSearchInteracted();
      removeSavedPreset(id);
      if (selection.source !== "saved" || selection.id !== id) return;
      const fallback = firstPresetSelection(selection.kind, prPresets, issuePresets);
      setProgrammaticSelection(fallback.selection);
      setQueryImmediate(fallback.filter);
      setRepoFilter("");
    },
    [
      markSearchInteracted,
      removeSavedPreset,
      issuePresets,
      prPresets,
      selection.id,
      selection.kind,
      selection.source,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
    ],
  );

  const onToggleSavedDefault = useCallback(
    async (preset: SavedPreset) => {
      if (defaultMutationPendingRef.current) return;
      defaultMutationPendingRef.current = true;
      setDefaultMutationPending(true);
      try {
        const defaultId = preset.isDefault ? null : preset.id;
        const persisted = await setSavedPresetDefault(preset.kind, defaultId);
        if (persisted) markSearchInteracted();
      } catch {
        toast({
          description: t("integrations:failedToUpdateDefaultView"),
          variant: "error",
        });
      } finally {
        defaultMutationPendingRef.current = false;
        setDefaultMutationPending(false);
      }
    },
    [markSearchInteracted, setSavedPresetDefault, t, toast],
  );

  return {
    savedPresets,
    onConfirmSave,
    onDeleteSaved,
    onToggleSavedDefault,
    defaultMutationPending,
  };
}
