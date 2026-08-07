"use client";

import { useCallback, useEffect, useState, type RefObject, type ReactNode } from "react";
import {
  IconChevronDown,
  IconCloudDownload,
  IconFolderPlus,
  IconGitBranch,
  IconPlus,
  IconStack2,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Input } from "@kandev/ui/input";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useBranchesByURL } from "@/hooks/domains/github/use-branches-by-url";
import { usePRInfoByURL } from "@/hooks/domains/github/use-pr-info-by-url";
import { useRemoteRepositories } from "@/hooks/domains/integrations/use-remote-repositories";
import { FolderPicker } from "@/components/folder-picker";
import { RemoteRepoChip } from "@/components/task-create-dialog-remote-repo-chip";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import type { LocalRepository, Repository } from "@/lib/types/http";
import { type WorkspaceSourceRow } from "@/components/workspace-source-picker/workspace-source-state";
import {
  getWorkspaceSourceCapabilities,
  hasCloneableSavedRepository,
} from "@/components/workspace-source-picker/executor-capabilities";
import { SavedRepositorySourceRow } from "./saved-repository-source-row";
import { useDialogOpenerFocus } from "./use-dialog-opener-focus";
import { useSubmitWorkspaceSources } from "./use-submit-workspace-sources";
import { useWorkspaceRepositoryOptions } from "./use-workspace-repository-options";
import { useWorkspaceSourceRows } from "./use-workspace-source-rows";
import { WorkspaceChangeConsequences } from "./workspace-change-consequences";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  executorType?: string | null;
  workspaceId: string | null;
  /** The toolbar button that explicitly opened this controlled surface. */
  opener?: HTMLElement | null;
  openerRef?: RefObject<HTMLButtonElement | null>;
};

export function AddWorkspaceSourcesDialog({
  open,
  onOpenChange,
  taskId,
  executorType,
  workspaceId,
  opener,
  openerRef,
}: Props) {
  const { isMobile } = useResponsiveBreakpoint();
  const { repositories, discoveredRepositories, repositoriesRefreshing, refreshRepositoryOptions } =
    useWorkspaceRepositoryOptions(workspaceId, open);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const sourceRows = useWorkspaceSourceRows(executorType);
  const reconcileWorkspaceSourcesAdopted = useAppStore(
    (state) => state.reconcileWorkspaceSourcesAdopted,
  );
  const { requestFocusRestoration, restoreOpenerFocus } = useDialogOpenerFocus({
    open,
    opener,
    openerRef,
  });
  const capabilities = getWorkspaceSourceCapabilities(executorType);
  const restartsWorkspace = !isRemoteWorkspaceExecutor(executorType);
  const close = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen && !submitting) {
        requestFocusRestoration();
        sourceRows.resetValidation();
        setSubmitError(null);
        onOpenChange(false);
      }
    },
    [onOpenChange, requestFocusRestoration, sourceRows, submitting],
  );
  const submit = useSubmitWorkspaceSources({
    errors: sourceRows.errors,
    onOpenChange,
    reconcileWorkspaceSourcesAdopted,
    rows: sourceRows.rows,
    submitting,
    taskId,
    onSuccess: () => {
      sourceRows.reset();
      requestFocusRestoration();
    },
    setSubmitting,
    setSubmitError,
  });
  return (
    <AddWorkspaceSourcesSurface
      isMobile={isMobile}
      open={open}
      onOpenChange={close}
      onCloseAutoFocus={restoreOpenerFocus}
      onDrawerCloseAnimationEnd={restoreOpenerFocus}
      error={submitError}
      consequences={<WorkspaceChangeConsequences restartsWorkspace={restartsWorkspace} />}
      form={
        <SourceForm
          rows={sourceRows.rows}
          workspaceId={workspaceId}
          errors={sourceRows.visibleErrors}
          repositories={selectableRepositories(repositories, capabilities)}
          discoveredRepositories={
            capabilities.requiresCloneableLocalRepository ? [] : discoveredRepositories
          }
          repositoriesRefreshing={repositoriesRefreshing}
          onRefreshRepositories={refreshRepositoryOptions}
          capabilities={capabilities}
          onAdd={(kind) => {
            setSubmitError(null);
            sourceRows.add(kind);
          }}
          onRemove={(key) => {
            setSubmitError(null);
            sourceRows.remove(key);
          }}
          onUpdate={(key, patch) => {
            setSubmitError(null);
            sourceRows.update(key, patch);
          }}
          isMobile={isMobile}
        />
      }
      submitting={submitting}
      canSubmit={sourceRows.rows.length > 0}
      onCancel={() => close(false)}
      onSubmit={() => {
        sourceRows.validate();
        void submit();
      }}
    />
  );
}

function selectableRepositories(
  repositories: Repository[],
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>,
) {
  return capabilities.requiresCloneableLocalRepository
    ? repositories.filter(hasCloneableSavedRepository)
    : repositories;
}

function isRemoteWorkspaceExecutor(executorType: string | null | undefined): boolean {
  return ["local_docker", "remote_docker", "ssh", "sprites"].includes(executorType ?? "");
}

type AddWorkspaceSourcesSurfaceProps = {
  isMobile: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus: (event?: { preventDefault(): void }) => void;
  onDrawerCloseAnimationEnd: (event: { preventDefault(): void }) => void;
  error: string | null;
  consequences: ReactNode;
  form: ReactNode;
  submitting: boolean;
  canSubmit: boolean;
  onCancel: () => void;
  onSubmit: () => void;
};

function AddWorkspaceSourcesSurface({
  isMobile,
  open,
  onOpenChange,
  onCloseAutoFocus,
  onDrawerCloseAnimationEnd,
  error,
  consequences,
  form,
  submitting,
  canSubmit,
  onCancel,
  onSubmit,
}: AddWorkspaceSourcesSurfaceProps) {
  const { t } = useTranslation();
  const footer = (
    <div className="flex justify-end gap-2">
      <Button
        type="button"
        variant="outline"
        className="min-h-11 cursor-pointer"
        disabled={submitting}
        onClick={onCancel}
      >
        {t("common:cancel")}
      </Button>
      <Button
        type="button"
        data-testid="add-workspace-sources-submit"
        className="min-h-11 cursor-pointer"
        disabled={submitting || !canSubmit}
        onClick={onSubmit}
      >
        {submitting ? t("task:adding") : t("task:addToWorkspace")}
      </Button>
    </div>
  );
  const errorMessage = error && (
    <p role="alert" className="text-sm text-destructive">
      {error}
    </p>
  );
  if (isMobile)
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          data-testid="add-workspace-sources-drawer"
          onCloseAutoFocus={onCloseAutoFocus}
          onAnimationEnd={(event) => {
            if (event.currentTarget.dataset.state === "closed") onDrawerCloseAnimationEnd(event);
          }}
          className="h-dvh !max-h-dvh rounded-none flex flex-col overflow-hidden data-[vaul-drawer-direction=bottom]:!mt-0"
        >
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{t("task:addToWorkspace")}</DrawerTitle>
            <DrawerDescription>{t("task:chooseRepositoriesOrFoldersToMake")}</DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4">
            {errorMessage}
            <div className="mb-4">{consequences}</div>
            {form}
          </div>
          <div className="shrink-0 border-t p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
            {footer}
          </div>
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="add-workspace-sources-dialog"
        className="flex max-h-[calc(100dvh-2rem)] max-w-xl flex-col overflow-hidden"
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <DialogHeader className="shrink-0">
          <DialogTitle>{t("task:addToWorkspace")}</DialogTitle>
          <DialogDescription>{t("task:chooseRepositoriesOrFoldersToMake")}</DialogDescription>
        </DialogHeader>
        <div
          data-testid="add-workspace-sources-dialog-scroll"
          className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain pr-1"
        >
          {errorMessage}
          {consequences}
          {form}
        </div>
        <DialogFooter className="shrink-0">{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SourceForm({
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
  isMobile: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4 py-1" data-testid="add-workspace-sources-form">
      <div className="flex flex-wrap items-center gap-2">
        <RepositorySourceMenu isMobile={isMobile} onAdd={onAdd} />
        {capabilities.canAddFolders && (
          <Button
            type="button"
            variant="outline"
            className={cn("cursor-pointer", isMobile ? "min-h-11" : "h-9 px-3")}
            onClick={() => onAdd("folder")}
          >
            <IconFolderPlus className="h-4 w-4" />
            {t("task:addFolder")}
          </Button>
        )}
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
