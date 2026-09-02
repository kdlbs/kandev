"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Issue, MR } from "@/lib/types/gitlab";
import type { SidebarSelection } from "@/components/gitlab/my-gitlab/presets-sidebar";
import { MR_PRESETS, ISSUE_PRESETS, presetLabel } from "@/components/gitlab/my-gitlab/presets";
import { useGitLabSearch } from "@/components/gitlab/my-gitlab/use-gitlab-search";
import { useSavedPresets, type SavedPreset } from "@/components/gitlab/my-gitlab/use-saved-presets";
import { useKnownProjects } from "@/components/gitlab/my-gitlab/use-known-projects";
import { useCommittedQuery } from "@/components/gitlab/my-gitlab/use-committed-query";

// Normative trim set for the milestone filter: Unicode White_Space plus
// U+0085 (NEL), which JS's built-in String.prototype.trim() and the bare \s
// regex class both omit. Mirrors the backend's trimGitLabWhitespace. Every
// commit site for the milestone draft must go through this helper, never
// String.prototype.trim() directly.
export function trimGitLabMilestone(value: string): string {
  return value.replace(/^[\s\u0085]+|[\s\u0085]+$/gu, "");
}

function resolveTitle(
  selection: SidebarSelection,
  saved: SavedPreset[],
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  if (selection.source === "saved") {
    return saved.find((p) => p.id === selection.id)?.label ?? t("gitlab:savedQueryFallback");
  }
  const presets = selection.kind === "mr" ? MR_PRESETS : ISSUE_PRESETS;
  return (
    presetLabel(
      t,
      presets.find((p) => p.value === selection.id),
    ) ?? (selection.kind === "mr" ? t("gitlab:titleMergeRequests") : t("gitlab:titleIssues"))
  );
}

const EMPTY_PROJECTS: string[] = [];

export type UseProjectOptionsArgs = {
  selection: SidebarSelection;
  committedQuery: string;
  milestone: string;
  items: Array<MR | Issue>;
  loading: boolean;
  projectFilter: string;
};

/** Encode every reset-key component as a JSON tuple so user values cannot collide. */
export function buildProjectOptionsResetKey(
  selection: SidebarSelection,
  committedQuery: string,
  milestone: string,
): string {
  return JSON.stringify([
    selection.kind,
    selection.source,
    selection.id,
    committedQuery.trim(),
    milestone,
  ]);
}

export function useProjectOptions({
  selection,
  committedQuery,
  milestone,
  items,
  loading,
  projectFilter,
}: UseProjectOptionsArgs): string[] {
  const resetKey = buildProjectOptionsResetKey(selection, committedQuery, milestone);

  // `useKnownProjects`'s accumulator clears the instant `resetKey` changes and
  // immediately refills with whatever project list it is handed in that same
  // effect call. On the render where a key change first lands, `items` (from
  // useGitLabSearch) still holds the *previous* key's results, and `loading`
  // still reads false, because the fetch for the new key is dispatched from
  // an effect that has not run yet (state.loading only flips inside
  // fetchData, which runs from an effect). `committedKey` mirrors `resetKey`
  // one render late via an ordinary effect, so on that first render
  // `committedKey !== resetKey` forces the page list empty regardless of
  // what `loading` happens to read at that instant; by the time
  // `committedKey` catches up, the fetch has been dispatched and `loading`
  // governs the empty state correctly for the rest of the request.
  const [committedKey, setCommittedKey] = useState(resetKey);
  useEffect(() => {
    setCommittedKey(resetKey);
  }, [resetKey]);

  const pageProjects = useMemo(() => {
    if (loading || committedKey !== resetKey) return EMPTY_PROJECTS;
    return items.filter((it) => !!it.project_path).map((it) => it.project_path);
  }, [loading, committedKey, resetKey, items]);

  const knownProjects = useKnownProjects(resetKey, pageProjects);
  return useMemo(() => {
    const set = new Set(knownProjects);
    if (projectFilter) set.add(projectFilter);
    return Array.from(set).sort();
  }, [knownProjects, projectFilter]);
}

/** Lifted out of `useGitLabPageState`, which is at the 100-line function cap. */
export function initialSelection(): SidebarSelection {
  return { kind: "mr", source: "preset", id: MR_PRESETS[0]?.value ?? "" };
}

type SaveQueryDialogArgs = {
  selection: SidebarSelection;
  setSelection: (s: SidebarSelection) => void;
  committedQuery: string;
  projectFilter: string;
  committedMilestone: string;
  effectivePreset: string;
  saveSavedPreset: ReturnType<typeof useSavedPresets>["save"];
  removeSavedPreset: ReturnType<typeof useSavedPresets>["remove"];
  clearScope: () => void;
  resetPage: () => void;
};

/** Lifted out of `useGitLabPageState`, which is at the 100-line function cap. */
function useSaveQueryDialog({
  selection,
  setSelection,
  committedQuery,
  projectFilter,
  committedMilestone,
  effectivePreset,
  saveSavedPreset,
  removeSavedPreset,
  clearScope,
  resetPage,
}: SaveQueryDialogArgs) {
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);

  // Use committed values (not unflushed drafts) so the saved preset always
  // matches what is currently displayed in the list.
  const canSaveCurrent =
    committedQuery.trim().length > 0 || projectFilter.length > 0 || committedMilestone.length > 0;
  // i18n-exempt: persisted as the saved query's name, so it must not depend on the creating locale.
  const suggestedLabel =
    committedQuery.trim() ||
    (projectFilter ? `In ${projectFilter}` : "") ||
    committedMilestone ||
    "Saved query";

  const onOpenSaveDialog = useCallback(() => {
    if (canSaveCurrent) setSaveDialogOpen(true);
  }, [canSaveCurrent]);

  const onConfirmSave = useCallback(
    (label: string) => {
      const created = saveSavedPreset({
        kind: selection.kind,
        label,
        customQuery: committedQuery,
        projectFilter,
        milestone: committedMilestone,
        preset: effectivePreset,
      });
      setSelection({ kind: selection.kind, source: "saved", id: created.id });
    },
    [
      selection.kind,
      committedQuery,
      projectFilter,
      committedMilestone,
      effectivePreset,
      saveSavedPreset,
      setSelection,
    ],
  );

  const onDeleteSaved = useCallback(
    (id: string) => {
      removeSavedPreset(id);
      if (selection.source === "saved" && selection.id === id) {
        const fallbackPresets = selection.kind === "mr" ? MR_PRESETS : ISSUE_PRESETS;
        setSelection({
          kind: selection.kind,
          source: "preset",
          id: fallbackPresets[0]?.value ?? "",
        });
        clearScope();
        resetPage();
      }
    },
    [removeSavedPreset, selection, setSelection, clearScope, resetPage],
  );

  return {
    saveDialogOpen,
    setSaveDialogOpen,
    canSaveCurrent,
    suggestedLabel,
    onOpenSaveDialog,
    onConfirmSave,
    onDeleteSaved,
  };
}

type ScopeActionsArgs = {
  savedPresets: SavedPreset[];
  setSelection: (s: SidebarSelection) => void;
  setQueryImmediate: (v: string) => void;
  setProjectFilter: (v: string) => void;
  setMilestoneImmediate: (v: string) => void;
  milestone: string;
  setPage: (p: number) => void;
};

/** Lifted out of `useGitLabPageState`, which is at the 100-line function cap. */
function useScopeActions({
  savedPresets,
  setSelection,
  setQueryImmediate,
  setProjectFilter,
  setMilestoneImmediate,
  milestone,
  setPage,
}: ScopeActionsArgs) {
  const onSelect = useCallback(
    (s: SidebarSelection) => {
      setSelection(s);
      if (s.source === "saved") {
        const found = savedPresets.find((p) => p.id === s.id);
        setQueryImmediate(found?.customQuery ?? "");
        setProjectFilter(found?.projectFilter ?? "");
        setMilestoneImmediate(found?.milestone ?? "");
        setPage(1);
        return;
      }
      setQueryImmediate("");
      setProjectFilter("");
      setMilestoneImmediate("");
      setPage(1);
    },
    [
      savedPresets,
      setSelection,
      setQueryImmediate,
      setProjectFilter,
      setMilestoneImmediate,
      setPage,
    ],
  );

  // Every path that changes the committed milestone calls setPage(1) from
  // inside the same event handler (commit, select, delete), so a stale later
  // page is never left showing results for a filter that no longer applies —
  // a saved query or sidebar preset can differ from the current state only
  // in its milestone, in which case useGitLabSearch's own
  // `[preset, customQuery, kind]` reset effect would not otherwise fire.
  const onCommitMilestone = useCallback(() => {
    setMilestoneImmediate(trimGitLabMilestone(milestone));
    setPage(1);
  }, [milestone, setMilestoneImmediate, setPage]);

  const clearScope = useCallback(() => {
    setQueryImmediate("");
    setProjectFilter("");
    setMilestoneImmediate("");
  }, [setQueryImmediate, setProjectFilter, setMilestoneImmediate]);

  const resetPage = useCallback(() => setPage(1), [setPage]);

  return { onSelect, onCommitMilestone, clearScope, resetPage };
}

type SearchAndProjectsArgs = {
  searchEnabled: boolean;
  workspaceId?: string;
  selection: SidebarSelection;
  committedQuery: string;
  committedMilestone: string;
  projectFilter: string;
  savedPresets: SavedPreset[];
  t: (key: string, values?: Record<string, unknown>) => string;
};

/** Lifted out of `useGitLabPageState`, which is at the 100-line function cap. */
function useSearchAndProjects({
  searchEnabled,
  workspaceId,
  selection,
  committedQuery,
  committedMilestone,
  projectFilter,
  savedPresets,
  t,
}: SearchAndProjectsArgs) {
  const presets = selection.kind === "mr" ? MR_PRESETS : ISSUE_PRESETS;
  // A saved query with no custom query text still needs the preset filter it
  // was saved under, or restoring it would silently widen to "no filter".
  const effectivePreset = useMemo(() => {
    if (selection.source === "preset") return selection.id;
    return savedPresets.find((p) => p.id === selection.id)?.preset ?? "";
  }, [selection, savedPresets]);

  const search = useGitLabSearch({
    workspaceId: workspaceId ?? "",
    kind: selection.kind,
    presets,
    preset: effectivePreset,
    customQuery: committedQuery,
    milestone: committedMilestone,
    projectFilter,
    enabled: searchEnabled && Boolean(workspaceId),
  });
  const projectOptions = useProjectOptions({
    selection,
    committedQuery,
    milestone: committedMilestone,
    items: search.rawItems,
    loading: search.loading,
    projectFilter,
  });
  const title = useMemo(
    () => resolveTitle(selection, savedPresets, t),
    [selection, savedPresets, t],
  );

  return { search, projectOptions, title, effectivePreset };
}

export function useGitLabPageState(searchEnabled: boolean, workspaceId?: string) {
  const { t } = useTranslation();
  const [selection, setSelection] = useState<SidebarSelection>(initialSelection);
  const {
    draft: customQuery,
    committed: committedQuery,
    setDraft: setCustomQuery,
    setImmediate: setQueryImmediate,
    commit: commitCustomQuery,
  } = useCommittedQuery("");
  const {
    draft: milestone,
    committed: committedMilestone,
    setDraft: setMilestone,
    setImmediate: setMilestoneImmediate,
  } = useCommittedQuery("");
  const [projectFilter, setProjectFilter] = useState("");
  const {
    presets: savedPresets,
    save: saveSavedPreset,
    remove: removeSavedPreset,
  } = useSavedPresets();

  const { search, projectOptions, title, effectivePreset } = useSearchAndProjects({
    searchEnabled,
    workspaceId,
    selection,
    committedQuery,
    committedMilestone,
    projectFilter,
    savedPresets,
    t,
  });

  const { onSelect, onCommitMilestone, clearScope, resetPage } = useScopeActions({
    savedPresets,
    setSelection,
    setQueryImmediate,
    setProjectFilter,
    setMilestoneImmediate,
    milestone,
    setPage: search.setPage,
  });

  const {
    saveDialogOpen,
    setSaveDialogOpen,
    canSaveCurrent,
    suggestedLabel,
    onOpenSaveDialog,
    onConfirmSave,
    onDeleteSaved,
  } = useSaveQueryDialog({
    selection,
    setSelection,
    committedQuery,
    projectFilter,
    committedMilestone,
    effectivePreset,
    saveSavedPreset,
    removeSavedPreset,
    clearScope,
    resetPage,
  });

  return {
    selection,
    customQuery,
    committedQuery,
    setCustomQuery,
    commitCustomQuery,
    milestone,
    committedMilestone,
    setMilestone,
    onCommitMilestone,
    showMilestoneFilter: selection.kind === "issue",
    projectFilter,
    setProjectFilter,
    savedPresets,
    search,
    projectOptions,
    title,
    onSelect,
    canSaveCurrent,
    suggestedLabel,
    saveDialogOpen,
    setSaveDialogOpen,
    onOpenSaveDialog,
    onConfirmSave,
    onDeleteSaved,
  };
}

export type GitLabPageState = ReturnType<typeof useGitLabPageState>;
