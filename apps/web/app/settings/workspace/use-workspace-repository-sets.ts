"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";

import { createRepositorySet, deleteRepositorySet, updateRepositorySet } from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import { useAppStore } from "@/components/state-provider";
import type { RepositorySet } from "@/lib/types/http";

/**
 * An in-progress set edit. `setId` is null while creating.
 *
 * `members` is ordered, and that order is what applying the set will use, so
 * the editor's order is meaningful rather than incidental. A member's base is
 * optional; an empty value means that task creation should use its normal
 * default for that repository.
 */
export type RepositorySetDraftMember = {
  repositoryId: string;
  baseBranch: string;
};

export type RepositorySetDraft = {
  setId: string | null;
  name: string;
  description: string;
  members: RepositorySetDraftMember[];
};

/**
 * Create, edit, and delete state for the workspace repository-set section.
 *
 * Writes go straight to the API and then into the store, so the section reflects
 * the change without waiting for the `repository_set.*` event to arrive (which
 * still keeps other tabs current).
 */
export function useWorkspaceRepositorySets({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const upsertRepositorySet = useAppStore((state) => state.upsertRepositorySet);
  const removeRepositorySet = useAppStore((state) => state.removeRepositorySet);
  const [draft, setDraft] = useState<RepositorySetDraft | null>(null);
  const [deleting, setDeleting] = useState<RepositorySet | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const startCreate = useCallback(() => {
    setError(null);
    setDraft({ setId: null, name: "", description: "", members: [] });
  }, []);

  const startEdit = useCallback((set: RepositorySet) => {
    setError(null);
    setDraft({
      setId: set.id,
      name: set.name,
      description: set.description,
      members: set.repositories.map((member) => ({
        repositoryId: member.repository_id as string,
        baseBranch: member.base_branch ?? "",
      })),
    });
  }, []);

  const cancelEdit = useCallback(() => {
    setDraft(null);
    setError(null);
  }, []);

  const updateDraft = useCallback((patch: Partial<RepositorySetDraft>) => {
    setDraft((current) => (current ? { ...current, ...patch } : current));
  }, []);

  const submitDraft = useCallback(async () => {
    if (!draft || saving) return;
    const name = draft.name.trim();
    if (!name || draft.members.length === 0) return;
    setSaving(true);
    setError(null);
    try {
      // A supplied repositories list replaces the whole membership list, which
      // is also how the editor expresses a reorder and base change.
      const repositories = draft.members.map((member) => ({
        repositoryId: member.repositoryId,
        baseBranch: member.baseBranch,
      }));
      const saved = draft.setId
        ? await updateRepositorySet(draft.setId, {
            name,
            description: draft.description.trim(),
            repositories,
          })
        : await createRepositorySet(workspaceId, {
            name,
            description: draft.description.trim(),
            repositories,
          });
      upsertRepositorySet(workspaceId, saved);
      setDraft(null);
    } catch (caught) {
      setError(editorErrorMessage(caught, t));
    } finally {
      setSaving(false);
    }
  }, [draft, saving, workspaceId, upsertRepositorySet, t]);

  const startDelete = useCallback((set: RepositorySet) => {
    setError(null);
    setDeleting(set);
  }, []);
  const cancelDelete = useCallback(() => setDeleting(null), []);

  const confirmDelete = useCallback(async () => {
    if (!deleting) return;
    setError(null);
    try {
      await deleteRepositorySet(deleting.id);
      removeRepositorySet(workspaceId, deleting.id);
      setDeleting(null);
    } catch (caught) {
      // The row stays, so silence would read as "the list is stale" and invite a
      // retry against a set that may already be gone. Keep the dialog open with
      // the reason.
      setError(deleteErrorMessage(caught, t));
    }
  }, [deleting, workspaceId, removeRepositorySet, t]);

  return {
    draft,
    deleting,
    error,
    saving,
    startCreate,
    startEdit,
    cancelEdit,
    updateDraft,
    submitDraft,
    startDelete,
    cancelDelete,
    confirmDelete,
  };
}

/**
 * A duplicate name is the one failure the user can fix in place, and the backend
 * message already names the existing set, so it is shown as-is.
 */
function editorErrorMessage(caught: unknown, t: (key: string) => string): string {
  if (caught instanceof ApiError && caught.status === 409) {
    return apiErrorBodyMessage(caught) ?? t("workspaces:repositorySetsNameTaken");
  }
  if (caught instanceof ApiError && caught.status === 422) {
    return apiErrorBodyMessage(caught) ?? t("workspaces:repositorySetsMemberRejected");
  }
  return t("workspaces:repositorySetsSaveFailed");
}

/** A set already deleted elsewhere is not a failure the user must act on. */
function deleteErrorMessage(caught: unknown, t: (key: string) => string): string {
  if (caught instanceof ApiError && caught.status === 404) {
    return t("workspaces:repositorySetsAlreadyDeleted");
  }
  return t("workspaces:repositorySetsDeleteFailed");
}

function apiErrorBodyMessage(caught: ApiError): string | null {
  if (typeof caught.body !== "object" || caught.body === null) return null;
  const message = (caught.body as { error?: unknown }).error;
  return typeof message === "string" && message !== "" ? message : null;
}
