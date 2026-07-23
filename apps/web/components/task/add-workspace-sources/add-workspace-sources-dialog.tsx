"use client";

import { useCallback, useEffect, useRef, useState, type RefObject, type ReactNode } from "react";
import { IconPlus, IconX } from "@tabler/icons-react";
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
import { Input } from "@kandev/ui/input";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { ControlledSourceModeSwitch } from "@/components/task-create-dialog-source-mode";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useBranchesByURL } from "@/hooks/domains/github/use-branches-by-url";
import { usePRInfoByURL } from "@/hooks/domains/github/use-pr-info-by-url";
import { useRemoteRepositories } from "@/hooks/domains/integrations/use-remote-repositories";
import { FolderPicker } from "@/components/folder-picker";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import { RemoteRepoChip } from "@/components/task-create-dialog-remote-repo-chip";
import type { TaskRepoRow, TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import {
  addWorkspaceSourceRow,
  getWorkspaceSourceValidation,
  removeWorkspaceSourceRow,
  updateWorkspaceSourceRow,
  type WorkspaceSourceRow,
} from "@/components/workspace-source-picker/workspace-source-state";
import {
  getWorkspaceSourceCapabilities,
  hasCloneableSavedRepository,
} from "@/components/workspace-source-picker/executor-capabilities";
import { useSubmitWorkspaceSources } from "./use-submit-workspace-sources";

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
  const { repositories } = useRepositories(workspaceId, open);
  const [rows, setRows] = useState<WorkspaceSourceRow[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const reconcileWorkspaceSourcesAdopted = useAppStore(
    (state) => state.reconcileWorkspaceSourcesAdopted,
  );
  const capturedOpenerRef = useRef<HTMLElement | null>(null);
  const shouldRestoreFocusRef = useRef(false);
  if (open && opener) capturedOpenerRef.current = opener;
  const errors = getWorkspaceSourceValidation(rows);
  const capabilities = getWorkspaceSourceCapabilities(executorType);
  const restoreOpenerFocus = useCallback((event?: { preventDefault(): void }) => {
    if (!shouldRestoreFocusRef.current) return;
    const focusTarget = openerRef?.current ?? capturedOpenerRef.current;
    event?.preventDefault();
    if (!focusTarget?.isConnected || focusTarget.matches(":disabled")) {
      shouldRestoreFocusRef.current = false;
      capturedOpenerRef.current = null;
      return;
    }
    shouldRestoreFocusRef.current = false;
    capturedOpenerRef.current = null;
    focusTarget.focus();
  }, []);
  const close = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen && !submitting) {
        shouldRestoreFocusRef.current = true;
        onOpenChange(false);
      }
    },
    [onOpenChange, restoreOpenerFocus, submitting],
  );
  useEffect(() => {
    if (!open) restoreOpenerFocus();
  }, [open, restoreOpenerFocus]);
  const add = useCallback(
    (kind: NonNullable<WorkspaceSourceRow["sourceType"]>) =>
      setRows((current) => addWorkspaceSourceRow(current, kind, executorType)),
    [executorType],
  );
  const update = useCallback(
    (key: string, patch: Partial<WorkspaceSourceRow>) =>
      setRows((current) => updateWorkspaceSourceRow(current, key, patch)),
    [],
  );
  const submit = useSubmitWorkspaceSources({
    errors,
    onOpenChange,
    reconcileWorkspaceSourcesAdopted,
    rows,
    submitting,
    taskId,
    onSuccess: () => {
      setRows([]);
      shouldRestoreFocusRef.current = true;
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
      form={
        <SourceForm
          rows={rows}
          workspaceId={workspaceId}
          errors={errors}
          repositories={selectableRepositories(repositories, capabilities)}
          capabilities={capabilities}
          onAdd={add}
          onRemove={(key) => setRows((current) => removeWorkspaceSourceRow(current, key))}
          onUpdate={update}
          isMobile={isMobile}
        />
      }
      submitting={submitting}
      onCancel={() => close(false)}
      onSubmit={() => void submit()}
    />
  );
}

function selectableRepositories(
  repositories: Parameters<typeof WorkspaceRepoChips>[0]["repositories"],
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>,
) {
  return capabilities.requiresCloneableLocalRepository
    ? repositories.filter(hasCloneableSavedRepository)
    : repositories;
}

type AddWorkspaceSourcesSurfaceProps = {
  isMobile: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus: (event?: { preventDefault(): void }) => void;
  onDrawerCloseAnimationEnd: (event: { preventDefault(): void }) => void;
  error: string | null;
  form: ReactNode;
  submitting: boolean;
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
  form,
  submitting,
  onCancel,
  onSubmit,
}: AddWorkspaceSourcesSurfaceProps) {
  const footer = (
    <div className="flex justify-end gap-2">
      <Button
        type="button"
        variant="outline"
        className="min-h-11 cursor-pointer"
        disabled={submitting}
        onClick={onCancel}
      >
        Cancel
      </Button>
      <Button
        type="button"
        data-testid="add-workspace-sources-submit"
        className="min-h-11 cursor-pointer"
        disabled={submitting}
        onClick={onSubmit}
      >
        {submitting ? "Adding…" : "Add sources"}
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
            <DrawerTitle>Add sources</DrawerTitle>
            <DrawerDescription>Add repositories or folders to this task.</DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4">
            {errorMessage}
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
        className="max-w-xl"
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <DialogHeader>
          <DialogTitle>Add sources</DialogTitle>
          <DialogDescription>Add repositories or folders to this task.</DialogDescription>
        </DialogHeader>
        {errorMessage}
        {form}
        <DialogFooter>{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SourceForm({
  rows,
  repositories,
  workspaceId,
  errors,
  capabilities,
  onAdd,
  onRemove,
  onUpdate,
  isMobile,
}: {
  rows: WorkspaceSourceRow[];
  repositories: Parameters<typeof WorkspaceRepoChips>[0]["repositories"];
  workspaceId: string | null;
  errors: Record<string, string>;
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  onAdd: (kind: NonNullable<WorkspaceSourceRow["sourceType"]>) => void;
  onRemove: (key: string) => void;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
  isMobile: boolean;
}) {
  const [mode, setMode] = useState<"local" | "remote">("local");
  return (
    <div className="space-y-3 py-2" data-testid="add-workspace-sources-form">
      <div className="flex flex-wrap items-center gap-2">
        <ControlledSourceModeSwitch
          mode={mode}
          onModeChange={setMode}
          touchSized={isMobile}
          options={[
            { value: "local", label: "Local", testId: "source-mode-local" },
            { value: "remote", label: "Remote", testId: "source-mode-remote" },
          ]}
        />
        <div className="flex flex-wrap gap-2">
          {mode === "local" && (
            <>
              <AddButton label="Saved repository" onClick={() => onAdd("saved_repository")} />
              <AddButton label="Local Git repository" onClick={() => onAdd("local_repository")} />
              {capabilities.canAddFolders && (
                <AddButton label="Local folder" onClick={() => onAdd("folder")} />
              )}
            </>
          )}
          {mode === "remote" && (
            <AddButton label="Remote Git repository" onClick={() => onAdd("remote_repository")} />
          )}
        </div>
      </div>
      {capabilities.requiresCloneableLocalRepository && (
        <p className="text-sm text-muted-foreground">
          Saved and local Git repositories must have a cloneable origin for this executor. Local
          folders are unavailable.
        </p>
      )}
      {rows.map((row) => (
        <SourceRow
          key={row.key}
          row={row}
          repositories={repositories}
          workspaceId={workspaceId}
          capabilities={capabilities}
          error={errors[row.key]}
          onRemove={onRemove}
          onUpdate={onUpdate}
        />
      ))}
    </div>
  );
}

function AddButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <Button type="button" variant="outline" className="min-h-11 cursor-pointer" onClick={onClick}>
      <IconPlus className="mr-1 h-4 w-4" />
      {label}
    </Button>
  );
}

function SourceRow({
  row,
  repositories,
  workspaceId,
  capabilities,
  error,
  onRemove,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  repositories: Parameters<typeof WorkspaceRepoChips>[0]["repositories"];
  workspaceId: string | null;
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  error?: string;
  onRemove: (key: string) => void;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const type = row.sourceType ?? (row.kind === "folder" ? "folder" : "saved_repository");
  return (
    <fieldset className="space-y-2 rounded border p-3" data-testid="workspace-source-row">
      <div className="flex items-center justify-between">
        <legend className="text-sm font-medium">{labelFor(type)}</legend>
        <button
          type="button"
          aria-label="Remove source"
          className="min-h-11 min-w-11 cursor-pointer text-muted-foreground"
          onClick={() => onRemove(row.key)}
        >
          <IconX className="mx-auto h-4 w-4" />
        </button>
      </div>
      {type === "saved_repository" && (
        <SavedRepositoryRow
          row={row}
          repositories={repositories}
          workspaceId={workspaceId}
          canChooseCheckoutBranch={capabilities.canChooseCheckoutBranch}
          onUpdate={onUpdate}
        />
      )}
      {type === "local_repository" && (
        <LocalPathRow
          row={row}
          label="Choose local Git repository"
          canChooseCheckoutBranch={capabilities.canChooseCheckoutBranch}
          requiresCloneableOrigin={capabilities.requiresCloneableLocalRepository}
          onUpdate={onUpdate}
        />
      )}
      {type === "remote_repository" && (
        <RemoteRepositoryRow
          row={row}
          workspaceId={workspaceId}
          canChooseCheckoutBranch={capabilities.canChooseCheckoutBranch}
          onUpdate={onUpdate}
        />
      )}
      {type === "folder" && (
        <LocalPathRow row={row} label="Choose local folder" onUpdate={onUpdate} />
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </fieldset>
  );
}

function labelFor(type: NonNullable<WorkspaceSourceRow["sourceType"]>) {
  return type.replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function SavedRepositoryRow({
  row,
  repositories,
  workspaceId,
  canChooseCheckoutBranch,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  repositories: Parameters<typeof WorkspaceRepoChips>[0]["repositories"];
  workspaceId: string | null;
  canChooseCheckoutBranch: boolean;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const chip: TaskRepoRow = {
    key: row.key,
    repositoryId: row.repositoryId,
    branch: row.baseBranch ?? "",
  };
  return (
    <>
      <WorkspaceRepoChips
        rows={[chip]}
        repositories={repositories}
        workspaceId={workspaceId}
        canAddMore={false}
        onAdd={() => {}}
        onRemove={() => {}}
        onRowRepositoryChange={(_, repositoryId) =>
          onUpdate(row.key, { repositoryId, localPath: undefined, remoteUrl: undefined })
        }
        onRowBranchChange={(_, baseBranch) => onUpdate(row.key, { baseBranch })}
      />
      {canChooseCheckoutBranch ? <CheckoutBranchInput row={row} onUpdate={onUpdate} /> : null}
    </>
  );
}

function LocalPathRow({
  row,
  label,
  canChooseCheckoutBranch = false,
  requiresCloneableOrigin = false,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  label: string;
  canChooseCheckoutBranch?: boolean;
  requiresCloneableOrigin?: boolean;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
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
          aria-label="Folder display name"
          placeholder="Display name (optional)"
          value={row.displayName ?? ""}
          onChange={(event) => onUpdate(row.key, { displayName: event.target.value })}
        />
      )}
      {row.sourceType === "local_repository" && (
        <>
          <Input
            aria-label="Base branch"
            placeholder="Base branch"
            value={row.baseBranch ?? ""}
            onChange={(event) => onUpdate(row.key, { baseBranch: event.target.value })}
          />
          <p className="text-sm text-muted-foreground">
            {requiresCloneableOrigin
              ? "This repository must have a cloneable origin; Kandev will verify it before adding."
              : "Uses the current checkout. Kandev does not switch your local repository branch."}
          </p>
          {canChooseCheckoutBranch ? <CheckoutBranchInput row={row} onUpdate={onUpdate} /> : null}
        </>
      )}
    </>
  );
}

function RemoteRepositoryRow({
  row,
  workspaceId,
  canChooseCheckoutBranch,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  workspaceId: string | null;
  canChooseCheckoutBranch: boolean;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const branches = useBranchesByURL();
  const prInfo = usePRInfoByURL();
  const accessibleRepos = useRemoteRepositories(workspaceId ?? "");
  useEffect(() => {
    if (row.remoteUrl) branches.ensure(row.remoteUrl, workspaceId ?? undefined);
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
      {canChooseCheckoutBranch ? <CheckoutBranchInput row={row} onUpdate={onUpdate} /> : null}
    </>
  );
}

function CheckoutBranchInput({
  row,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  return (
    <Input
      aria-label="Checkout branch"
      placeholder="Checkout branch (optional)"
      value={row.checkoutBranch ?? ""}
      onChange={(event) => onUpdate(row.key, { checkoutBranch: event.target.value })}
    />
  );
}
