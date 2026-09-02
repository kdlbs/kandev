"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { IconPencil, IconStack2, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { SettingsSection } from "@/components/settings/settings-section";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { Repository, RepositorySet } from "@/lib/types/http";
import { useRepositorySets } from "@/hooks/domains/workspace/use-repository-sets";
import { useWorkspaceRepositorySets } from "./use-workspace-repository-sets";
import { RepositorySetEditorDialog } from "./workspace-repository-set-editor";
import { RepositorySetDeleteConfirmation } from "./workspace-repository-set-delete";

type WorkspaceRepositorySetsSectionProps = {
  workspaceId: string;
  repositories: Repository[];
  readOnly: boolean;
};

/**
 * Manages this workspace's repository sets, below the repository list.
 *
 * Sets live here rather than on a tab of their own: they are a grouping of the
 * repositories on this very page, and a new tab would mean touching the tab
 * table, the settings route matcher, the discovery catalog, and their tests for a
 * section with one natural home.
 */
export function WorkspaceRepositorySetsSection({
  workspaceId,
  repositories,
  readOnly,
}: WorkspaceRepositorySetsSectionProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const { sets } = useRepositorySets(workspaceId);
  const manager = useWorkspaceRepositorySets({ workspaceId });
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null);

  useEffect(() => {
    if (!confirmingDeleteId || sets.some((set) => set.id === confirmingDeleteId)) return;
    setConfirmingDeleteId(null);
    manager.cancelDelete();
  }, [confirmingDeleteId, manager, sets]);

  const startDelete = (set: RepositorySet) => {
    manager.startDelete(set);
    setConfirmingDeleteId(set.id);
  };

  const cancelDelete = () => {
    setConfirmingDeleteId(null);
    manager.cancelDelete();
  };

  const closeDeleteConfirmation = () => {
    setConfirmingDeleteId(null);
  };

  const confirmDelete = () => {
    setConfirmingDeleteId(null);
    void manager.confirmDelete();
  };

  return (
    <SettingsSection
      divided
      icon={<IconStack2 className="h-5 w-5" />}
      title={t("workspaces:repositorySetsTitle")}
      description={t("workspaces:repositorySetsDescription")}
      action={
        readOnly ? undefined : (
          <Button
            size="sm"
            className="cursor-pointer"
            onClick={() => manager.startCreate()}
            data-testid="repository-set-create"
          >
            {t("workspaces:repositorySetsCreate")}
          </Button>
        )
      }
    >
      {sets.length === 0 ? (
        <p className="text-sm text-muted-foreground" data-testid="repository-sets-empty">
          {t("workspaces:repositorySetsEmpty")}
        </p>
      ) : (
        <div className="grid gap-3">
          {sets.map((set) => (
            <RepositorySetRow
              key={set.id}
              set={set}
              repositories={repositories}
              readOnly={readOnly}
              isFinePointer={isFinePointer}
              confirmingDelete={confirmingDeleteId === set.id}
              deleteError={manager.deleting?.id === set.id ? manager.error : null}
              onEdit={() => manager.startEdit(set)}
              onDelete={() => startDelete(set)}
              onDeleteCancel={cancelDelete}
              onDeleteOpenChange={closeDeleteConfirmation}
              onDeleteConfirm={confirmDelete}
            />
          ))}
        </div>
      )}
      <RepositorySetEditorDialog
        workspaceId={workspaceId}
        draft={manager.draft}
        repositories={repositories}
        error={manager.error}
        saving={manager.saving}
        onClose={manager.cancelEdit}
        onChange={manager.updateDraft}
        onSubmit={manager.submitDraft}
      />
    </SettingsSection>
  );
}

type RepositorySetRowProps = {
  set: RepositorySet;
  repositories: Repository[];
  readOnly: boolean;
  isFinePointer: boolean;
  confirmingDelete: boolean;
  deleteError: string | null;
  onEdit: () => void;
  onDelete: () => void;
  onDeleteCancel: () => void;
  onDeleteOpenChange: (open: boolean) => void;
  onDeleteConfirm: () => void;
};

/**
 * One set: name, description, and its members as chips. A set whose repositories
 * were all deleted renders with no chips rather than disappearing, matching the
 * contract that such a set is kept.
 */
function RepositorySetRow({
  set,
  repositories,
  readOnly,
  isFinePointer,
  confirmingDelete,
  deleteError,
  onEdit,
  onDelete,
  onDeleteCancel,
  onDeleteOpenChange,
  onDeleteConfirm,
}: RepositorySetRowProps) {
  const { t } = useTranslation();
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const namesById = new Map(
    repositories.map((repository) => [repository.id as string, repository]),
  );

  return (
    <div
      className="rounded-md border p-3"
      data-testid="repository-set-row"
      data-member-count={set.repositories.length}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">{set.name}</p>
          {set.description ? (
            <p className="text-xs text-muted-foreground">{set.description}</p>
          ) : null}
        </div>
        <RepositorySetRowActions
          set={set}
          readOnly={readOnly}
          isFinePointer={isFinePointer}
          confirmingDelete={confirmingDelete}
          deleteAnchorRef={deleteAnchorRef}
          onEdit={onEdit}
          onDelete={onDelete}
        />
      </div>
      {!isFinePointer && confirmingDelete ? (
        <InlineConfirmActions
          density="touch"
          testId="repository-set-delete-inline-confirmation"
          ariaLabel={t("workspaces:repositorySetsDeleteTitle", { name: set.name })}
          description={t("workspaces:repositorySetsDeleteDescription")}
          cancelLabel={t("common:cancel")}
          confirmLabel={t("workspaces:repositorySetsDelete")}
          confirmAriaLabel={t("workspaces:repositorySetsDeleteTitle", { name: set.name })}
          confirmTestId="repository-set-delete-confirm"
          onCancel={onDeleteCancel}
          onClose={() => onDeleteOpenChange(false)}
          onConfirm={onDeleteConfirm}
        />
      ) : null}
      {deleteError ? (
        <p className="mt-2 text-xs text-destructive" data-testid="repository-set-delete-error">
          {deleteError}
        </p>
      ) : null}
      <div className="mt-2 flex flex-wrap gap-1.5">
        {set.repositories.length === 0 ? (
          <span className="text-xs text-muted-foreground">
            {t("workspaces:repositorySetsNoMembers")}
          </span>
        ) : (
          set.repositories.map((member) => (
            <Badge key={member.repository_id} variant="secondary">
              <span>
                {namesById.get(member.repository_id as string)?.name ?? member.repository_id}
              </span>
              {member.base_branch ? (
                <span
                  className="ml-1 text-muted-foreground"
                  data-testid={`repository-set-member-base-${member.repository_id}`}
                >
                  {t("workspaces:repositorySetsMemberBase", { base: member.base_branch })}
                </span>
              ) : null}
            </Badge>
          ))
        )}
      </div>
      {isFinePointer ? (
        <RepositorySetDeleteConfirmation
          set={set}
          open={confirmingDelete}
          anchorRef={deleteAnchorRef}
          onOpenChange={onDeleteOpenChange}
          onCancel={onDeleteCancel}
          onConfirm={onDeleteConfirm}
        />
      ) : null}
    </div>
  );
}

type RepositorySetRowActionsProps = {
  set: RepositorySet;
  readOnly: boolean;
  isFinePointer: boolean;
  confirmingDelete: boolean;
  deleteAnchorRef: RefObject<HTMLButtonElement | null>;
  onEdit: () => void;
  onDelete: () => void;
};

function RepositorySetRowActions({
  set,
  readOnly,
  isFinePointer,
  confirmingDelete,
  deleteAnchorRef,
  onEdit,
  onDelete,
}: RepositorySetRowActionsProps) {
  const { t } = useTranslation();
  if (readOnly) return null;
  // Fine pointers keep the Delete button as the popover anchor. Coarse
  // pointers replace that button with the row-owned inline confirmation.
  if (!isFinePointer && confirmingDelete) return null;
  return (
    <div className="flex shrink-0 gap-1">
      <Button
        size="sm"
        variant="ghost"
        className="cursor-pointer"
        aria-label={t("workspaces:repositorySetsEdit")}
        onClick={onEdit}
        data-testid={`repository-set-edit-${set.id}`}
      >
        <IconPencil className="h-4 w-4" />
      </Button>
      <Button
        ref={deleteAnchorRef}
        size="sm"
        variant="ghost"
        className="cursor-pointer"
        aria-label={t("workspaces:repositorySetsDelete")}
        onClick={onDelete}
        data-testid={`repository-set-delete-${set.id}`}
      >
        <IconTrash className="h-4 w-4" />
      </Button>
    </div>
  );
}
