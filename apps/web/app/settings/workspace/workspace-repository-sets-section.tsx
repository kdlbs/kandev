"use client";

import { IconPencil, IconStack2, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

import { SettingsSection } from "@/components/settings/settings-section";
import type { Repository, RepositorySet } from "@/lib/types/http";
import { useRepositorySets } from "@/hooks/domains/workspace/use-repository-sets";
import { RepositorySetEditorDialog } from "./workspace-repository-set-editor";
import { RepositorySetDeleteDialog } from "./workspace-repository-set-delete";
import { useWorkspaceRepositorySets } from "./use-workspace-repository-sets";

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
  const { sets } = useRepositorySets(workspaceId);
  const manager = useWorkspaceRepositorySets({ workspaceId });

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
              onEdit={() => manager.startEdit(set)}
              onDelete={() => manager.startDelete(set)}
            />
          ))}
        </div>
      )}
      <RepositorySetEditorDialog
        draft={manager.draft}
        repositories={repositories}
        error={manager.error}
        saving={manager.saving}
        onClose={manager.cancelEdit}
        onChange={manager.updateDraft}
        onSubmit={manager.submitDraft}
      />
      <RepositorySetDeleteDialog
        set={manager.deleting}
        error={manager.deleting ? manager.error : null}
        onClose={manager.cancelDelete}
        onConfirm={manager.confirmDelete}
      />
    </SettingsSection>
  );
}

type RepositorySetRowProps = {
  set: RepositorySet;
  repositories: Repository[];
  readOnly: boolean;
  onEdit: () => void;
  onDelete: () => void;
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
  onEdit,
  onDelete,
}: RepositorySetRowProps) {
  const { t } = useTranslation();
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
        {readOnly ? null : (
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
        )}
      </div>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {set.repositories.length === 0 ? (
          <span className="text-xs text-muted-foreground">
            {t("workspaces:repositorySetsNoMembers")}
          </span>
        ) : (
          set.repositories.map((member) => (
            <Badge key={member.repository_id} variant="secondary">
              {namesById.get(member.repository_id as string)?.name ?? member.repository_id}
            </Badge>
          ))
        )}
      </div>
    </div>
  );
}
