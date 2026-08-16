"use client";

import { FormEvent, useEffect, useState } from "react";
import {
  IconAlertCircle,
  IconFolder,
  IconFolderOpen,
  IconFolderPlus,
  IconInfoCircle,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import { Input } from "@kandev/ui/input";
import { DirectoryBrowserBody, useDirectoryListing } from "@/components/folder-picker";
import type { DirectLocalExecutorSelection } from "@/components/task-create-dialog-handlers";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { initializeLocalRepository } from "@/lib/api/domains/workspace-api";
import { createDirectory } from "@/lib/api/domains/fs-api";
import type { Repository } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { t } from "@/lib/i18n";

// The returned strings are never rendered: the only caller uses this as a
// boolean gate (`!nameError`), so they stay English rather than becoming dead
// catalog entries.
export function validateLocalRepositoryName(name: string): string | null {
  const trimmed = name.trim();
  if (!trimmed) return t("workspaces:enterRepositoryName");
  if (trimmed === "." || trimmed === "..") return t("workspaces:chooseDifferentRepositoryName");
  if (trimmed.includes("/") || trimmed.includes("\\") || trimmed.includes("\0")) {
    return t("workspaces:repositoryNameMustBeOneFolder");
  }
  return null;
}

export function buildLocalRepositoryTargetPath(parentPath: string, name: string): string {
  const separator = parentPath.includes("\\") && !parentPath.includes("/") ? "\\" : "/";
  const parent = parentPath.replace(/[\\/]+$/, "");
  if (!parent && separator === "/") return `/${name.trim()}`;
  return `${parent}${separator}${name.trim()}`;
}

export type CreateLocalRepositorySurfaceProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  executorSelection: DirectLocalExecutorSelection | null;
  context?: "task-create" | "workspace";
  onCreated: (repository: Repository) => void;
};

// `context` and `requiresSwitch` stay logic; only the notices they select are
// copy. The profile name is user data, interpolated rather than translated.
function executorNotice(
  t: TFunction,
  selection: DirectLocalExecutorSelection | null,
  context: NonNullable<CreateLocalRepositorySurfaceProps["context"]>,
): string {
  if (context === "workspace") {
    return t("common:createsAnEmptyGitRepository");
  }
  if (!selection) {
    return t("common:aDirectLocalExecutorProfileIsRequired");
  }
  if (selection.requiresSwitch) {
    return t("common:emptyRepositoriesRunDirectlySwitch", {
      profile: selection.executorProfileName,
    });
  }
  return t("common:thisEmptyRepositoryWillRunWith", {
    profile: selection.executorProfileName,
  });
}

type RepositoryLocationFieldsProps = {
  name: string;
  parentPath: string;
  targetPath: string;
  onNameChange: (name: string) => void;
  onParentPathChange: (path: string) => void;
  onLoadTypedDirectory: () => void;
};

function RepositoryLocationFields({
  name,
  parentPath,
  targetPath,
  onNameChange,
  onParentPathChange,
  onLoadTypedDirectory,
}: RepositoryLocationFieldsProps) {
  const { t } = useTranslation();
  return (
    <>
      <label className="block space-y-1.5 text-xs font-medium">
        <span>{t("common:repositoryName")}</span>
        <Input
          value={name}
          onChange={(event) => onNameChange(event.target.value)}
          placeholder="new-project"
          autoFocus
          className="h-11 sm:h-8"
        />
      </label>
      <div className="space-y-1.5">
        <label htmlFor="local-repository-parent" className="text-xs font-medium">
          {t("common:parentDirectory")}
        </label>
        <div className="flex min-w-0 items-center gap-2">
          <Input
            id="local-repository-parent"
            value={parentPath}
            onChange={(event) => onParentPathChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key !== "Enter") return;
              event.preventDefault();
              onLoadTypedDirectory();
            }}
            placeholder="/Users/you/Projects"
            className="h-11 min-w-0 flex-1 font-mono sm:h-8"
          />
          <Button
            type="button"
            variant="outline"
            size="icon-lg"
            className="size-11 sm:size-8"
            onClick={onLoadTypedDirectory}
            disabled={!parentPath}
            aria-label={t("common:browseParentDirectory")}
            title={t("common:browseParentDirectory")}
          >
            <IconFolderOpen />
          </Button>
        </div>
      </div>
      <div className="flex min-w-0 items-center gap-2 border-t border-border/70 pt-2 sm:col-span-2">
        <IconFolder className="size-4 shrink-0 text-muted-foreground" />
        <span className="shrink-0 text-xs text-muted-foreground">{t("common:destination")}</span>
        <span className="truncate font-mono text-xs" title={targetPath || parentPath}>
          {targetPath || parentPath || t("common:loadingFolder")}
        </span>
      </div>
    </>
  );
}

type RepositoryFormDetailsProps = RepositoryLocationFieldsProps & {
  executorSelection: DirectLocalExecutorSelection | null;
  context: NonNullable<CreateLocalRepositorySurfaceProps["context"]>;
  submitError: string | null;
};

function RepositoryFormDetails({
  executorSelection,
  context,
  submitError,
  ...locationFields
}: RepositoryFormDetailsProps) {
  const { t } = useTranslation();
  return (
    <div className="shrink-0 space-y-3 px-4 py-4">
      <div className="grid gap-3 sm:grid-cols-[minmax(0,0.8fr)_minmax(0,1.6fr)]">
        <RepositoryLocationFields {...locationFields} />
      </div>
      <div
        className={
          executorSelection || context === "workspace"
            ? "flex items-start gap-2 text-xs text-muted-foreground"
            : "flex items-start gap-2 text-xs text-destructive"
        }
      >
        <IconInfoCircle className="mt-0.5 size-3.5 shrink-0" />
        <p>{executorNotice(t, executorSelection, context)}</p>
      </div>
      {submitError ? (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive"
        >
          <IconAlertCircle className="mt-0.5 size-3.5 shrink-0" />
          <p>{submitError}</p>
        </div>
      ) : null}
    </div>
  );
}

function CreateRepositoryFooter({
  canSubmit,
  submitting,
}: {
  canSubmit: boolean;
  submitting: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 justify-end border-t border-border px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
      <Button
        type="submit"
        className="min-h-11 w-full cursor-pointer sm:w-auto sm:min-w-40"
        disabled={!canSubmit}
      >
        <IconFolderPlus className="h-4 w-4" />
        {submitting ? t("common:creating") : t("common:createRepository")}
      </Button>
    </div>
  );
}

function CreateRepositoryForm({
  open,
  workspaceId,
  executorSelection,
  context = "task-create",
  onCreated,
  onDismiss,
}: CreateLocalRepositorySurfaceProps & { onDismiss: () => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [parentPath, setParentPath] = useState("");
  const [editingParentPath, setEditingParentPath] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const { listing, loading, error: listingError, load } = useDirectoryListing(open, "");
  useEffect(() => {
    if (!open) {
      setParentPath("");
      setEditingParentPath(false);
      setSubmitError(null);
      return;
    }
    if (listing && !editingParentPath) setParentPath(listing.path);
  }, [editingParentPath, listing, open]);

  const nameError = validateLocalRepositoryName(name);
  const targetPath =
    parentPath && !nameError ? buildLocalRepositoryTargetPath(parentPath, name) : "";
  const executorReady = Boolean(executorSelection || context === "workspace");
  const canSubmit = !!workspaceId && executorReady && !!parentPath && !nameError && !submitting;

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    event.stopPropagation();
    if (!canSubmit || !workspaceId) return;
    setSubmitting(true);
    setSubmitError(null);
    try {
      const repository = await initializeLocalRepository(workspaceId, {
        name: name.trim(),
        parentPath,
      });
      onCreated(repository);
      setName("");
      onDismiss();
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : t("common:failedToCreateRepository"));
    } finally {
      setSubmitting(false);
    }
  };

  const handleDirectoryNavigation = (path: string) => {
    setSubmitError(null);
    setParentPath(path);
    setEditingParentPath(false);
    void load(path);
  };

  const loadTypedDirectory = () => {
    if (parentPath) void load(parentPath);
  };

  const handleParentPathChange = (path: string) => {
    setSubmitError(null);
    setParentPath(path);
    setEditingParentPath(true);
  };

  const handleCreateDirectory = async (folderName: string) => {
    if (!listing) return;
    setSubmitError(null);
    const created = await createDirectory(listing.path, folderName);
    setParentPath(created.path);
    setEditingParentPath(false);
    await load(created.path);
  };

  return (
    <form onSubmit={(event) => void handleSubmit(event)} className="flex min-h-0 flex-1 flex-col">
      <RepositoryFormDetails
        name={name}
        parentPath={parentPath}
        targetPath={targetPath}
        executorSelection={executorSelection}
        context={context}
        submitError={submitError}
        onNameChange={(nextName) => {
          setSubmitError(null);
          setName(nextName);
        }}
        onParentPathChange={handleParentPathChange}
        onLoadTypedDirectory={loadTypedDirectory}
      />
      <DirectoryBrowserBody
        listing={listing}
        loading={loading}
        error={listingError}
        onNavigate={handleDirectoryNavigation}
        onCreateDirectory={handleCreateDirectory}
        touchRows
        fillAvailableHeight
      />
      <CreateRepositoryFooter canSubmit={canSubmit} submitting={submitting} />
    </form>
  );
}

export function CreateLocalRepositorySurface(props: CreateLocalRepositorySurfaceProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const handleOpenChange = (open: boolean) => props.onOpenChange(open);
  const form = <CreateRepositoryForm {...props} onDismiss={() => handleOpenChange(false)} />;

  if (isMobile) {
    return (
      <Drawer open={props.open} onOpenChange={handleOpenChange}>
        <DrawerContent
          data-testid="create-local-repository-drawer"
          className="h-[88dvh] max-h-[88dvh] min-w-0 overflow-hidden pb-0"
        >
          <DrawerHeader className="shrink-0 border-b border-border text-left">
            <DrawerTitle>{t("common:createNewRepository")}</DrawerTitle>
            <DrawerDescription>{t("common:chooseAFolderOrEnterA")}</DrawerDescription>
          </DrawerHeader>
          {form}
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent
        data-testid="create-local-repository-dialog"
        className="flex h-[min(640px,85dvh)] max-w-xl min-w-0 flex-col overflow-hidden p-0"
      >
        <DialogHeader className="shrink-0 border-b border-border px-4 py-3 pr-12">
          <DialogTitle>{t("common:createNewRepository")}</DialogTitle>
          <DialogDescription>{t("common:chooseAFolderOrEnterA")}</DialogDescription>
        </DialogHeader>
        {form}
      </DialogContent>
    </Dialog>
  );
}
