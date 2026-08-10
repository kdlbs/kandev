"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";
import type { PresetOption } from "./search-bar";
import type { SidebarSelection } from "./presets-sidebar";
import {
  findDefaultSavedPreset,
  resolveDefaultSidebarTarget,
  type SavedPreset,
} from "./saved-preset-model";

type InitialSidebarSelectionOptions = {
  workspaceId: string | null;
  resolvedPrPresets: PresetOption[];
  autoResetSearchRef: MutableRefObject<boolean>;
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  savedPresets?: SavedPreset[];
};

type AppliedSidebarTarget = ReturnType<typeof resolveDefaultSidebarTarget> & {
  workspaceId: string | null;
};

function sameAppliedSidebarTarget(
  current: AppliedSidebarTarget | null,
  next: AppliedSidebarTarget,
) {
  return (
    current !== null &&
    current.workspaceId === next.workspaceId &&
    current.selection.kind === next.selection.kind &&
    current.selection.source === next.selection.source &&
    current.selection.id === next.selection.id &&
    current.query === next.query &&
    current.repoFilter === next.repoFilter
  );
}

export function useInitialSidebarSelection({
  workspaceId,
  resolvedPrPresets,
  autoResetSearchRef,
  setQueryImmediate,
  setRepoFilter,
  savedPresets = [],
}: InitialSidebarSelectionOptions) {
  const userSelectedRef = useRef(false);
  const resetWorkspaceIdRef = useRef<string | null | undefined>(undefined);
  const lastAppliedTargetRef = useRef<AppliedSidebarTarget | null>(null);
  const savedPrDefault = useMemo(() => findDefaultSavedPreset(savedPresets, "pr"), [savedPresets]);
  const resolvedPrTarget = useMemo(
    () =>
      resolveDefaultSidebarTarget("pr", savedPrDefault ? [savedPrDefault] : [], resolvedPrPresets),
    [savedPrDefault, resolvedPrPresets],
  );
  const [selection, setSelection] = useState<SidebarSelection>(() => resolvedPrTarget.selection);

  useEffect(() => {
    if (resetWorkspaceIdRef.current !== workspaceId) {
      resetWorkspaceIdRef.current = workspaceId;
      userSelectedRef.current = false;
      // Re-enable default selection for the new workspace even after prior interaction.
      autoResetSearchRef.current = true;
    }
    if (userSelectedRef.current || !autoResetSearchRef.current) return;
    const target = resolvedPrTarget;
    const appliedTarget = { workspaceId, ...target };
    if (sameAppliedSidebarTarget(lastAppliedTargetRef.current, appliedTarget)) return;
    lastAppliedTargetRef.current = appliedTarget;
    setSelection((current) =>
      current.kind === target.selection.kind &&
      current.source === target.selection.source &&
      current.id === target.selection.id
        ? current
        : target.selection,
    );
    setQueryImmediate(target.query);
    setRepoFilter(target.repoFilter);
  }, [workspaceId, resolvedPrTarget, setQueryImmediate, setRepoFilter]);

  const setUserSelection = useCallback((next: SidebarSelection) => {
    userSelectedRef.current = true;
    setSelection(next);
  }, []);

  return { selection, setProgrammaticSelection: setSelection, setUserSelection };
}

export function useSidebarSelectionHandler({
  savedPresets,
  resolvedPrPresets,
  resolvedIssuePresets,
  setQueryImmediate,
  setRepoFilter,
  setUserSelection,
  markSearchInteracted,
  currentKind,
}: {
  savedPresets: SavedPreset[];
  resolvedPrPresets: PresetOption[];
  resolvedIssuePresets: PresetOption[];
  setQueryImmediate: (query: string) => void;
  setRepoFilter: (repo: string) => void;
  setUserSelection: (next: SidebarSelection) => void;
  markSearchInteracted: () => void;
  currentKind: SidebarSelection["kind"];
}) {
  const savedPresetsRef = useRef(savedPresets);
  savedPresetsRef.current = savedPresets;
  return useCallback(
    (selection: SidebarSelection) => {
      // The empty preset id is only meaningful when the kind toggle changes kinds.
      if (selection.kind === currentKind && selection.source === "preset" && selection.id === "") {
        return;
      }
      markSearchInteracted();
      // Kind-switch: always resolve from the saved default (or first configured preset),
      // not from the specific id in `selection`, which is just the kind-toggle fallback.
      if (selection.kind !== currentKind) {
        const target = resolveDefaultSidebarTarget(
          selection.kind,
          savedPresetsRef.current,
          selection.kind === "pr" ? resolvedPrPresets : resolvedIssuePresets,
        );
        setUserSelection(target.selection);
        setQueryImmediate(target.query);
        setRepoFilter(target.repoFilter);
        return;
      }
      setUserSelection(selection);
      if (selection.source === "saved") {
        const found = savedPresetsRef.current.find((preset) => preset.id === selection.id);
        setQueryImmediate(found?.customQuery ?? "");
        setRepoFilter(found?.repoFilter ?? "");
        return;
      }
      const preset = (selection.kind === "pr" ? resolvedPrPresets : resolvedIssuePresets).find(
        (candidate) => candidate.value === selection.id,
      );
      setQueryImmediate(preset?.filter ?? "");
      setRepoFilter("");
    },
    [
      setQueryImmediate,
      resolvedPrPresets,
      resolvedIssuePresets,
      setUserSelection,
      markSearchInteracted,
      setRepoFilter,
      currentKind,
    ],
  );
}
