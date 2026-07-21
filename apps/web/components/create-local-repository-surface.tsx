"use client";

import { FormEvent, useState } from "react";
import { IconFolderPlus } from "@tabler/icons-react";
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
import type { Repository } from "@/lib/types/http";

export function validateLocalRepositoryName(name: string): string | null {
  const trimmed = name.trim();
  if (!trimmed) return "Enter a repository name.";
  if (trimmed === "." || trimmed === "..") return "Choose a different repository name.";
  if (trimmed.includes("/") || trimmed.includes("\\")) {
    return "The repository name must be one folder name.";
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
  onCreated: (repository: Repository) => void;
};

function executorNotice(selection: DirectLocalExecutorSelection | null): string {
  if (!selection) {
    return "A direct local executor profile is required to create and use an empty repository.";
  }
  if (selection.requiresSwitch) {
    return `Empty repositories run directly on this machine. This task will switch to “${selection.executorProfileName}”.`;
  }
  return `This empty repository will run with “${selection.executorProfileName}” on this machine.`;
}

function CreateRepositoryForm({
  open,
  workspaceId,
  executorSelection,
  onCreated,
  onDismiss,
}: CreateLocalRepositorySurfaceProps & { onDismiss: () => void }) {
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const { listing, loading, error: listingError, load } = useDirectoryListing(open, "");
  const parentPath = listing?.path ?? "";
  const nameError = validateLocalRepositoryName(name);
  const targetPath =
    parentPath && !nameError ? buildLocalRepositoryTargetPath(parentPath, name) : "";
  const canSubmit = Boolean(
    workspaceId && executorSelection && parentPath && !nameError && !submitting,
  );

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    event.stopPropagation();
    if (!canSubmit || !workspaceId || !executorSelection) return;
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
      setSubmitError(err instanceof Error ? err.message : "Failed to create repository");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={(event) => void handleSubmit(event)} className="flex min-h-0 flex-1 flex-col">
      <div className="shrink-0 space-y-3 px-4 pb-3">
        <label className="block space-y-1.5 text-xs font-medium">
          <span>Repository name</span>
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="new-project"
            autoFocus
          />
        </label>
        <div className="min-w-0 rounded-md border border-border/70 bg-muted/30 px-3 py-2">
          <p className="text-[11px] text-muted-foreground">Repository path</p>
          <p className="truncate font-mono text-xs" title={targetPath || parentPath}>
            {targetPath || parentPath || "Loading folder…"}
          </p>
        </div>
        <p
          className={
            executorSelection ? "text-xs text-muted-foreground" : "text-xs text-destructive"
          }
        >
          {executorNotice(executorSelection)}
        </p>
        {submitError ? (
          <p role="alert" className="text-xs text-destructive">
            {submitError}
          </p>
        ) : null}
      </div>
      <DirectoryBrowserBody
        listing={listing}
        loading={loading}
        error={listingError}
        onNavigate={(path) => void load(path)}
        touchRows
        fillAvailableHeight
      />
      <div className="shrink-0 border-t border-border px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <Button type="submit" className="min-h-12 w-full cursor-pointer" disabled={!canSubmit}>
          <IconFolderPlus className="h-4 w-4" />
          {submitting ? "Creating…" : "Create repository"}
        </Button>
      </div>
    </form>
  );
}

export function CreateLocalRepositorySurface(props: CreateLocalRepositorySurfaceProps) {
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
            <DrawerTitle>Create new repository</DrawerTitle>
            <DrawerDescription>Choose an existing parent folder on this machine.</DrawerDescription>
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
        className="flex h-[min(680px,85dvh)] max-w-xl min-w-0 flex-col overflow-hidden p-0"
      >
        <DialogHeader className="shrink-0 border-b border-border px-4 py-3 pr-12">
          <DialogTitle>Create new repository</DialogTitle>
          <DialogDescription>Choose an existing parent folder on this machine.</DialogDescription>
        </DialogHeader>
        {form}
      </DialogContent>
    </Dialog>
  );
}
