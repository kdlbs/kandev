"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import type { PresetOption } from "./search-bar";
import type { SidebarSelection } from "./presets-sidebar";
import type { SavedPreset } from "./saved-preset-model";
import type { useSavedPresets } from "./use-saved-presets";

type SavedPresetStore = ReturnType<typeof useSavedPresets>;

type SavedPresetActionsOptions = {
  selection: SidebarSelection;
  customQuery: string;
  resolvedPrPresets: PresetOption[];
  resolvedIssuePresets: PresetOption[];
  setProgrammaticSelection: (selection: SidebarSelection) => void;
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  savedPresetStore: SavedPresetStore;
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

function useConfirmSave({
  kind,
  customQuery,
  save,
  markSearchInteracted,
  setProgrammaticSelection,
  setQueryImmediate,
  setRepoFilter,
  reportError,
}: {
  kind: SidebarSelection["kind"];
  customQuery: string;
  save: SavedPresetStore["save"];
  markSearchInteracted: () => void;
  setProgrammaticSelection: (selection: SidebarSelection) => void;
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  reportError: () => void;
}) {
  return useCallback(
    async (label: string, defaultRepoFilter: string) => {
      try {
        const created = await save({ kind, label, customQuery, repoFilter: defaultRepoFilter });
        if (!created) return;
        markSearchInteracted();
        setProgrammaticSelection({ kind, source: "saved", id: created.id });
        setQueryImmediate(customQuery);
        setRepoFilter(defaultRepoFilter);
      } catch {
        reportError();
      }
    },
    [
      customQuery,
      kind,
      markSearchInteracted,
      reportError,
      save,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
    ],
  );
}

function useDeleteSaved({
  selection,
  prPresets,
  issuePresets,
  remove,
  markSearchInteracted,
  setProgrammaticSelection,
  setQueryImmediate,
  setRepoFilter,
  reportError,
}: {
  selection: SidebarSelection;
  prPresets: PresetOption[];
  issuePresets: PresetOption[];
  remove: SavedPresetStore["remove"];
  markSearchInteracted: () => void;
  setProgrammaticSelection: (selection: SidebarSelection) => void;
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  reportError: () => void;
}) {
  const selectionRef = useRef(selection);
  selectionRef.current = selection;
  return useCallback(
    async (id: string) => {
      try {
        const removed = await remove(id);
        if (!removed) return;
        markSearchInteracted();
        const currentSelection = selectionRef.current;
        if (currentSelection.source === "saved" && currentSelection.id === id) {
          const fallback = firstPresetSelection(currentSelection.kind, prPresets, issuePresets);
          setProgrammaticSelection(fallback.selection);
          setQueryImmediate(fallback.filter);
          setRepoFilter("");
        }
      } catch {
        reportError();
      }
    },
    [
      issuePresets,
      markSearchInteracted,
      prPresets,
      remove,
      reportError,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
    ],
  );
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

  const reportSaveError = useCallback(
    () => toast({ description: t("integrations:failedToSaveSavedQuery"), variant: "error" }),
    [t, toast],
  );
  const reportDeleteError = useCallback(
    () => toast({ description: t("integrations:failedToDeleteSavedQuery"), variant: "error" }),
    [t, toast],
  );
  const onConfirmSave = useConfirmSave({
    kind: selection.kind,
    customQuery,
    save: saveSavedPreset,
    markSearchInteracted,
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
    reportError: reportSaveError,
  });
  const onDeleteSaved = useDeleteSaved({
    selection,
    prPresets,
    issuePresets,
    remove: removeSavedPreset,
    markSearchInteracted,
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
    reportError: reportDeleteError,
  });

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
