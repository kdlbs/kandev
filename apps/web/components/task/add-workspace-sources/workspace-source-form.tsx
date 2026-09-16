"use client";

import { useEffect, type ReactNode } from "react";
import {
  IconChevronDown,
  IconCloudDownload,
  IconGitBranch,
  IconPlus,
  IconStack2,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Input } from "@kandev/ui/input";
import { useBranchesByURL } from "@/hooks/domains/github/use-branches-by-url";
import { usePRInfoByURL } from "@/hooks/domains/github/use-pr-info-by-url";
import { useRemoteRepositories } from "@/hooks/domains/integrations/use-remote-repositories";
import { FolderPicker } from "@/components/folder-picker";
import { RemoteRepoChip } from "@/components/task-create-dialog-remote-repo-chip";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import type {
  LocalRepository,
  Repository,
  WorkspaceRepositoryPlacement,
  WorkspaceRepositoryPlacementPreview,
} from "@/lib/types/http";
import { type WorkspaceSourceRow } from "@/components/workspace-source-picker/workspace-source-state";
import { getWorkspaceSourceCapabilities } from "@/components/workspace-source-picker/executor-capabilities";
import { AddFolderButton } from "./add-folder-button";
import { SavedRepositorySourceRow } from "./saved-repository-source-row";
import { WorkspaceSourcePlacement } from "./workspace-source-placement";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

export function SourceForm({
  rows,
  repositories,
  discoveredRepositories,
  workspaceId,
  repositoriesRefreshing,
  onRefreshRepositories,
  errors,
  capabilities,
  onAdd,
  onRemove,
  onUpdate,
  repositoryPlacement,
  onRepositoryPlacementChange,
  placementPreview,
  placementPreviewError,
  placementPreviewLoading,
  isMobile,
}: {
  rows: WorkspaceSourceRow[];
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  workspaceId: string | null;
  repositoriesRefreshing: boolean;
  onRefreshRepositories: () => void;
  errors: Record<string, string>;
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  onAdd: (kind: NonNullable<WorkspaceSourceRow["sourceType"]>) => void;
  onRemove: (key: string) => void;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
  repositoryPlacement?: WorkspaceRepositoryPlacement | null;
  onRepositoryPlacementChange: (placement: WorkspaceRepositoryPlacement) => void;
  placementPreview?: WorkspaceRepositoryPlacementPreview | null;
  placementPreviewError?: string | null;
  placementPreviewLoading?: boolean;
  isMobile: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4 py-1" data-testid="add-workspace-sources-form">
      <div className="flex flex-wrap items-center gap-2">
        <RepositorySourceMenu isMobile={isMobile} onAdd={onAdd} />
        <AddFolderButton isMobile={isMobile} capabilities={capabilities} onAdd={onAdd} />
      </div>
      {capabilities.requiresCloneableLocalRepository && (
        <p className="text-sm text-muted-foreground">
          {t("task:savedAndLocalGitRepositoriesMust")}
        </p>
      )}
      {rows.map((row) => (
        <SourceRow
          key={row.key}
          row={row}
          repositories={repositories}
          discoveredRepositories={discoveredRepositories}
          workspaceId={workspaceId}
          repositoriesRefreshing={repositoriesRefreshing}
          onRefreshRepositories={onRefreshRepositories}
          capabilities={capabilities}
          error={errors[row.key]}
          onRemove={onRemove}
          onUpdate={onUpdate}
        />
      ))}
      {repositoryPlacement !== undefined && (
        <WorkspaceSourcePlacement
          hasRepositories={rows.some((row) => row.kind === "repository")}
          placement={repositoryPlacement}
          onPlacementChange={onRepositoryPlacementChange}
          preview={placementPreview}
          previewLoading={placementPreviewLoading}
        />
      )}
      {placementPreviewError && (
        <p role="alert" className="text-sm text-destructive">
          {placementPreviewError}
        </p>
      )}
    </div>
  );
}

function RepositorySourceMenu({
  isMobile,
  onAdd,
}: {
  isMobile: boolean;
  onAdd: (kind: "saved_repository" | "local_repository" | "remote_repository") => void;
}) {
  const { t } = useTranslation();
  const itemClass = cn("cursor-pointer items-start gap-3", isMobile ? "min-h-11" : "py-2");
  return (
    <DropdownMenu modal={!isMobile}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn("cursor-pointer", isMobile ? "min-h-11" : "h-9 px-3")}
        >
          <IconPlus className="h-4 w-4" />
          {t("task:addRepository")}
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-80 max-w-[calc(100vw-2rem)]">
        <RepositorySourceMenuItem
          label={t("task:workspaceRepository")}
          description={t("task:chooseFromSavedOrDiscoveredRepositories")}
          icon={<IconStack2 className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("saved_repository")}
        />
        <RepositorySourceMenuItem
          label={t("task:localGitRepository")}
          description={t("task:useAnExistingCheckoutOnThis")}
          icon={<IconGitBranch className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("local_repository")}
        />
        <RepositorySourceMenuItem
          label={t("task:remoteRepository")}
          description={t("task:cloneFromAProviderOrGit")}
          icon={<IconCloudDownload className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("remote_repository")}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function RepositorySourceMenuItem({
  label,
  description,
  icon,
  className,
  onSelect,
}: {
  label: string;
  description: string;
  icon: ReactNode;
  className: string;
  onSelect: () => void;
}) {
  return (
    <DropdownMenuItem aria-label={label} className={className} onSelect={onSelect}>
      {icon}
      <span className="min-w-0">
        <span className="block text-sm font-medium text-foreground">{label}</span>
        <span className="block text-xs text-muted-foreground">{description}</span>
      </span>
    </DropdownMenuItem>
  );
}

function SourceRow({
  row,
  repositories,
  discoveredRepositories,
  workspaceId,
  repositoriesRefreshing,
  onRefreshRepositories,
  capabilities,
  error,
  onRemove,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  workspaceId: string | null;
  repositoriesRefreshing: boolean;
  onRefreshRepositories: () => void;
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  error?: string;
  onRemove: (key: string) => void;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const { t } = useTranslation();
  const type = row.sourceType ?? (row.kind === "folder" ? "folder" : "saved_repository");
  return (
    <fieldset className="space-y-2 rounded border p-3" data-testid="workspace-source-row">
      <div className="flex items-center justify-between">
        <legend className="text-sm font-medium">{labelFor(type)}</legend>
        <button
          type="button"
          aria-label={t("task:removeSource")}
          className="min-h-11 min-w-11 cursor-pointer text-muted-foreground"
          onClick={() => onRemove(row.key)}
        >
          <IconX className="mx-auto h-4 w-4" />
        </button>
      </div>
      {type === "saved_repository" && (
        <SavedRepositorySourceRow
          row={row}
          repositories={repositories}
          discoveredRepositories={discoveredRepositories}
          workspaceId={workspaceId}
          canCreateRepository={!capabilities.requiresCloneableLocalRepository}
          repositoriesRefreshing={repositoriesRefreshing}
          onRefreshRepositories={onRefreshRepositories}
          onUpdate={onUpdate}
        />
      )}
      {type === "local_repository" && (
        <LocalPathRow
          row={row}
          label={t("task:chooseLocalGitRepository")}
          requiresCloneableOrigin={capabilities.requiresCloneableLocalRepository}
          onUpdate={onUpdate}
        />
      )}
      {type === "remote_repository" && (
        <RemoteRepositoryRow row={row} workspaceId={workspaceId} onUpdate={onUpdate} />
      )}
      {type === "folder" && (
        <LocalPathRow row={row} label={t("task:chooseLocalFolder")} onUpdate={onUpdate} />
      )}
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </fieldset>
  );
}

// Module-level `t` rather than a hook: this runs from render, after a locale is
// active. The `type` values are wire enums and stay in English.
function labelFor(type: NonNullable<WorkspaceSourceRow["sourceType"]>) {
  switch (type) {
    case "saved_repository":
      return t("task:workspaceRepository");
    case "local_repository":
      return t("task:localGitRepository");
    case "remote_repository":
      return t("task:remoteRepository");
    case "folder":
      return t("task:folder");
  }
}

function LocalPathRow({
  row,
  label,
  requiresCloneableOrigin = false,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  label: string;
  requiresCloneableOrigin?: boolean;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <FolderPicker
        value={row.localPath ?? ""}
        onChange={(localPath) =>
          onUpdate(row.key, { localPath, repositoryId: undefined, remoteUrl: undefined })
        }
        placeholder={label}
      />
      {row.sourceType === "folder" && (
        <Input
          aria-label={t("task:folderDisplayName")}
          placeholder={t("task:displayNameOptional")}
          value={row.displayName ?? ""}
          onChange={(event) => onUpdate(row.key, { displayName: event.target.value })}
        />
      )}
      {row.sourceType === "local_repository" && (
        <>
          <Input
            aria-label={t("task:baseBranch")}
            placeholder={t("task:baseBranch")}
            value={row.baseBranch ?? ""}
            onChange={(event) => onUpdate(row.key, { baseBranch: event.target.value })}
          />
          <p className="text-sm text-muted-foreground">
            {requiresCloneableOrigin
              ? t("task:thisRepositoryMustHaveACloneable")
              : t("task:usesTheCurrentCheckoutKandevDoes")}
          </p>
        </>
      )}
    </>
  );
}

function RemoteRepositoryRow({
  row,
  workspaceId,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  workspaceId: string | null;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const branches = useBranchesByURL(workspaceId);
  const prInfo = usePRInfoByURL(workspaceId);
  const accessibleRepos = useRemoteRepositories(workspaceId ?? "");
  useEffect(() => {
    if (row.remoteUrl) branches.ensure(row.remoteUrl);
  }, [branches, row.remoteUrl, workspaceId]);
  const remoteRow: TaskRemoteRepoRow = {
    key: row.key,
    url: row.remoteUrl ?? "",
    branch: row.baseBranch ?? "",
    source: "paste",
    provider: row.provider,
    providerRepoId: row.providerRepoId,
    providerOwner: row.providerOwner,
    providerName: row.providerName,
  };
  return (
    <>
      <RemoteRepoChip
        row={remoteRow}
        branches={branches.branches(remoteRow.url)}
        branchesLoading={branches.loading(remoteRow.url)}
        prInfo={prInfo.info(remoteRow.url)}
        accessibleRepos={accessibleRepos}
        onURLChange={(remoteUrl, _, metadata) =>
          onUpdate(row.key, {
            remoteUrl,
            repositoryId: undefined,
            localPath: undefined,
            provider: metadata?.provider,
            providerRepoId: metadata?.providerRepoId,
            providerOwner: metadata?.providerOwner,
            providerName: metadata?.providerName,
            baseBranch: metadata?.defaultBranch ?? "",
          })
        }
        onBranchChange={(baseBranch) => onUpdate(row.key, { baseBranch })}
        onRemove={() => {}}
      />
    </>
  );
}
