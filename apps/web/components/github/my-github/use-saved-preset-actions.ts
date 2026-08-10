"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import type { PresetOption } from "./search-bar";
import type { SidebarSelection } from "./presets-sidebar";
import type { SavedPreset } from "./saved-preset-model";
import type { useSavedPresets } from "./use-saved-presets";

type SavedPresetStore = ReturnType<typeof useSavedPresets>;
type WorkspaceId = string | null;
type IsCurrentWorkspace = (workspaceId: WorkspaceId) => boolean;

type SavedPresetActionsOptions = {
  workspaceId: string | null;
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

function useIsCurrentWorkspace(workspaceId: WorkspaceId): IsCurrentWorkspace {
  const workspaceIdRef = useRef(workspaceId);
  workspaceIdRef.current = workspaceId;
  return useCallback((candidate) => workspaceIdRef.current === candidate, []);
}

function useConfirmSave({
  workspaceId,
  kind,
  customQuery,
  save,
  markSearchInteracted,
  setProgrammaticSelection,
  setQueryImmediate,
  setRepoFilter,
  reportError,
  isCurrentWorkspace,
}: {
  workspaceId: WorkspaceId;
  kind: SidebarSelection["kind"];
  customQuery: string;
  save: SavedPresetStore["save"];
  markSearchInteracted: () => void;
  setProgrammaticSelection: (selection: SidebarSelection) => void;
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  reportError: () => void;
  isCurrentWorkspace: IsCurrentWorkspace;
}) {
  return useCallback(
    async (label: string, defaultRepoFilter: string) => {
      try {
        const created = await save({ kind, label, customQuery, repoFilter: defaultRepoFilter });
        // No persistence started when workspace presets are not available yet.
        if (!created) return;
        if (!isCurrentWorkspace(workspaceId)) return;
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
      isCurrentWorkspace,
      kind,
      markSearchInteracted,
      reportError,
      save,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      workspaceId,
    ],
  );
}

function useDeleteSaved({
  workspaceId,
  selection,
  prPresets,
  issuePresets,
  remove,
  markSearchInteracted,
  setProgrammaticSelection,
  setQueryImmediate,
  setRepoFilter,
  reportError,
  isCurrentWorkspace,
}: {
  workspaceId: WorkspaceId;
  selection: SidebarSelection;
  prPresets: PresetOption[];
  issuePresets: PresetOption[];
  remove: SavedPresetStore["remove"];
  markSearchInteracted: () => void;
  setProgrammaticSelection: (selection: SidebarSelection) => void;
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  reportError: () => void;
  isCurrentWorkspace: IsCurrentWorkspace;
}) {
  const selectionRef = useRef(selection);
  selectionRef.current = selection;
  const presetsRef = useRef({ prPresets, issuePresets });
  presetsRef.current = { prPresets, issuePresets };
  return useCallback(
    async (id: string) => {
      try {
        const removed = await remove(id);
        // Loading or a stale target is a no-op, not a persistence failure.
        if (!removed) return;
        if (!isCurrentWorkspace(workspaceId)) return;
        markSearchInteracted();
        const currentSelection = selectionRef.current;
        if (currentSelection.source === "saved" && currentSelection.id === id) {
          const fallback = firstPresetSelection(
            currentSelection.kind,
            presetsRef.current.prPresets,
            presetsRef.current.issuePresets,
          );
          setProgrammaticSelection(fallback.selection);
          setQueryImmediate(fallback.filter);
          setRepoFilter("");
        }
      } catch {
        reportError();
      }
    },
    [
      isCurrentWorkspace,
      markSearchInteracted,
      remove,
      reportError,
      setProgrammaticSelection,
      setQueryImmediate,
      setRepoFilter,
      workspaceId,
    ],
  );
}

export function useSavedPresetActions({
  workspaceId,
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
  const [, setDefaultMutationVersion] = useState(0);
  const pendingDefaultMutationsRef = useRef(new Map<WorkspaceId, symbol>());
  const isCurrentWorkspace = useIsCurrentWorkspace(workspaceId);
  const defaultMutationPending = pendingDefaultMutationsRef.current.has(workspaceId);
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
    workspaceId,
    kind: selection.kind,
    customQuery,
    save: saveSavedPreset,
    markSearchInteracted,
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
    reportError: reportSaveError,
    isCurrentWorkspace,
  });
  const onDeleteSaved = useDeleteSaved({
    workspaceId,
    selection,
    prPresets,
    issuePresets,
    remove: removeSavedPreset,
    markSearchInteracted,
    setProgrammaticSelection,
    setQueryImmediate,
    setRepoFilter,
    reportError: reportDeleteError,
    isCurrentWorkspace,
  });

  const onToggleSavedDefault = useCallback(
    async (preset: SavedPreset) => {
      const pendingMutations = pendingDefaultMutationsRef.current;
      if (pendingMutations.has(workspaceId)) return;
      const mutationToken = Symbol();
      pendingMutations.set(workspaceId, mutationToken);
      setDefaultMutationVersion((version) => version + 1);
      try {
        const defaultId = preset.isDefault ? null : preset.id;
        const persisted = await setSavedPresetDefault(preset.kind, defaultId);
        // Loading or an already-matching default is a no-op, not a persistence failure.
        if (!persisted) return;
        if (!isCurrentWorkspace(workspaceId)) return;
        // Mark interaction so the initial-selection effect does not switch the
        // active view to the newly set default on the same render cycle.
        markSearchInteracted();
      } catch {
        toast({
          description: t("integrations:failedToUpdateDefaultView"),
          variant: "error",
        });
      } finally {
        if (pendingMutations.get(workspaceId) === mutationToken) {
          pendingMutations.delete(workspaceId);
          if (isCurrentWorkspace(workspaceId)) {
            setDefaultMutationVersion((version) => version + 1);
          }
        }
      }
    },
    [isCurrentWorkspace, markSearchInteracted, setSavedPresetDefault, t, toast, workspaceId],
  );

  return {
    savedPresets,
    onConfirmSave,
    onDeleteSaved,
    onToggleSavedDefault,
    defaultMutationPending,
  };
}
