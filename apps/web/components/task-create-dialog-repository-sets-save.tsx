"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";

import { createRepositorySet } from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import { useAppStore } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import {
  selectedRepositoryIdsForSet,
  selectedRepositoryMembersForSet,
} from "@/components/task-create-dialog-repository-sets";

type SaveRepositorySetDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  /** The picker's current rows; only workspace repository rows can be saved. */
  rows: TaskRepoRow[];
  /** Workspace defaults are needed to capture the effective local base. */
  repositories?: Repository[];
  /** Local execution keeps the checkout branch separate from the base. */
  isLocalExecutor?: boolean;
};

/**
 * Saves the repositories currently chosen in the task form as a new set.
 *
 * This is the definition path that does not make the user leave a task they are
 * in the middle of creating: it creates the set and leaves the draft untouched.
 * Base choices are saved as part of each ordered member. Checkout state for a
 * local executor is intentionally not part of the set.
 */
export function SaveRepositorySetDialog({
  open,
  onOpenChange,
  workspaceId,
  rows,
  repositories,
  isLocalExecutor = false,
}: SaveRepositorySetDialogProps) {
  const { t } = useTranslation();
  const upsertRepositorySet = useAppStore((state) => state.upsertRepositorySet);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const repositoryIds = useMemo(() => selectedRepositoryIdsForSet(rows), [rows]);
  const repositoryMembers = useMemo(
    () => selectedRepositoryMembersForSet(rows, repositories, isLocalExecutor),
    [rows, repositories, isLocalExecutor],
  );
  // A row that names a discovered local path, a remote URL, or nothing at all is
  // not a workspace repository, so it cannot be a member.
  const excludedRowCount = rows.filter((row) => !row.repositoryId).length;

  const handleSubmit = async () => {
    const trimmed = name.trim();
    if (!trimmed || saving) return;
    setSaving(true);
    setError(null);
    try {
      const created = await createRepositorySet(workspaceId, {
        name: trimmed,
        description: description.trim(),
        repositories: repositoryMembers,
      });
      upsertRepositorySet(workspaceId, created);
      setName("");
      setDescription("");
      onOpenChange(false);
    } catch (caught) {
      setError(saveErrorMessage(caught, t));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("task:repositorySetsSaveTitle")}</DialogTitle>
          <DialogDescription>{t("task:repositorySetsSaveDescription")}</DialogDescription>
        </DialogHeader>
        {repositoryIds.length === 0 ? (
          <p className="text-sm text-muted-foreground" data-testid="repository-set-save-empty">
            {t("task:repositorySetsSaveNoSelection")}
          </p>
        ) : (
          <SaveRepositorySetFields
            name={name}
            description={description}
            memberCount={repositoryIds.length}
            excludedRowCount={excludedRowCount}
            error={error}
            onNameChange={setName}
            onDescriptionChange={setDescription}
          />
        )}
        <DialogFooter>
          {repositoryIds.length > 0 ? (
            <Button
              type="button"
              className="cursor-pointer"
              disabled={!name.trim() || saving}
              onClick={() => void handleSubmit()}
              data-testid="repository-set-save-submit"
            >
              {t("task:repositorySetsSaveSubmit")}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type SaveRepositorySetFieldsProps = {
  name: string;
  description: string;
  memberCount: number;
  excludedRowCount: number;
  error: string | null;
  onNameChange: (value: string) => void;
  onDescriptionChange: (value: string) => void;
};

/** The dialog's inputs and status lines, split out to keep the shell short. */
function SaveRepositorySetFields({
  name,
  description,
  memberCount,
  excludedRowCount,
  error,
  onNameChange,
  onDescriptionChange,
}: SaveRepositorySetFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <label className="block space-y-1.5 text-xs font-medium">
        <span>{t("task:repositorySetsSaveNameLabel")}</span>
        <Input
          value={name}
          onChange={(event) => onNameChange(event.target.value)}
          maxLength={100}
          autoFocus
          data-testid="repository-set-name"
        />
      </label>
      <label className="block space-y-1.5 text-xs font-medium">
        <span>{t("task:repositorySetsSaveDescriptionLabel")}</span>
        <Textarea
          value={description}
          onChange={(event) => onDescriptionChange(event.target.value)}
          rows={2}
          data-testid="repository-set-description"
        />
      </label>
      <p className="text-xs text-muted-foreground">
        {t("task:repositorySetsSaveMemberSummary", { count: memberCount })}
      </p>
      {excludedRowCount > 0 ? (
        <p className="text-xs text-muted-foreground" data-testid="repository-set-save-excluded">
          {t("task:repositorySetsSaveExcludedRows", { count: excludedRowCount })}
        </p>
      ) : null}
      {error ? (
        <p className="text-xs text-destructive" data-testid="repository-set-save-error">
          {error}
        </p>
      ) : null}
    </div>
  );
}

/**
 * A duplicate name is the one failure the user can act on directly, and the
 * backend's message already names the existing set, so it is shown as-is.
 */
function saveErrorMessage(caught: unknown, t: (key: string) => string): string {
  if (caught instanceof ApiError && caught.status === 409) {
    return errorBodyMessage(caught) ?? t("task:repositorySetsSaveNameTaken");
  }
  return t("task:repositorySetsSaveFailed");
}

function errorBodyMessage(caught: ApiError): string | null {
  if (typeof caught.body !== "object" || caught.body === null) return null;
  const message = (caught.body as { error?: unknown }).error;
  return typeof message === "string" && message !== "" ? message : null;
}
