"use client";

import { useCallback, useState, type RefObject, type ReactNode } from "react";
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
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";
import {
  getWorkspaceSourceCapabilities,
  hasCloneableSavedRepository,
} from "@/components/workspace-source-picker/executor-capabilities";
import { SourceForm } from "./workspace-source-form";
import { useDialogOpenerFocus } from "./use-dialog-opener-focus";
import { useSubmitWorkspaceSources } from "./use-submit-workspace-sources";
import { useWorkspaceRepositoryOptions } from "./use-workspace-repository-options";
import { useWorkspaceSourceRows } from "./use-workspace-source-rows";
import { useWorkspaceSourcePlacement } from "./use-workspace-source-placement";
import { WorkspaceChangeConsequences } from "./workspace-change-consequences";
import { useTranslation } from "react-i18next";

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

function useDialogWorkspacePlacement(
  open: boolean,
  taskId: string,
  executorType: string | null | undefined,
  sourceRows: ReturnType<typeof useWorkspaceSourceRows>,
) {
  const placement = useWorkspaceSourcePlacement({
    open,
    taskId,
    executorType,
    rows: sourceRows.rows,
    errors: sourceRows.errors,
  });
  return {
    ...placement,
    restartsWorkspace: workspaceChangeRestarts(placement.eligible, executorType),
  };
}

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
  const placementState = useDialogWorkspacePlacement(open, taskId, executorType, sourceRows);
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
    repositoryPlacement: placementState.eligible
      ? (placementState.placement ?? undefined)
      : undefined,
    previewRevision: placementState.eligible ? placementState.preview?.revision : undefined,
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
      consequences={
        <WorkspaceChangeConsequences restartsWorkspace={placementState.restartsWorkspace} />
      }
      form={
        <WorkspaceSourceForm
          sourceRows={sourceRows}
          workspaceId={workspaceId}
          repositories={selectableRepositories(repositories, capabilities)}
          repositoriesRefreshing={repositoriesRefreshing}
          onRefreshRepositories={refreshRepositoryOptions}
          capabilities={capabilities}
          discoveredRepositories={discoveredRepositories}
          placement={placementState}
          isMobile={isMobile}
          onClearError={() => setSubmitError(null)}
        />
      }
      submitting={submitting}
      canSubmit={canSubmitWorkspaceSources(sourceRows.rows.length, placementState)}
      onCancel={() => close(false)}
      onSubmit={() => {
        sourceRows.validate();
        void submit();
      }}
    />
  );
}

type WorkspaceSourceFormProps = {
  sourceRows: ReturnType<typeof useWorkspaceSourceRows>;
  workspaceId: string | null;
  repositories: Repository[];
  discoveredRepositories: ReturnType<
    typeof useWorkspaceRepositoryOptions
  >["discoveredRepositories"];
  repositoriesRefreshing: boolean;
  onRefreshRepositories: () => void;
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  placement: ReturnType<typeof useWorkspaceSourcePlacement>;
  isMobile: boolean;
  onClearError: () => void;
};

function WorkspaceSourceForm({
  sourceRows,
  workspaceId,
  repositories,
  discoveredRepositories,
  repositoriesRefreshing,
  onRefreshRepositories,
  capabilities,
  placement,
  isMobile,
  onClearError,
}: WorkspaceSourceFormProps) {
  return (
    <SourceForm
      rows={sourceRows.rows}
      workspaceId={workspaceId}
      errors={sourceRows.visibleErrors}
      repositories={repositories}
      discoveredRepositories={
        capabilities.requiresCloneableLocalRepository ? [] : discoveredRepositories
      }
      repositoriesRefreshing={repositoriesRefreshing}
      onRefreshRepositories={onRefreshRepositories}
      capabilities={capabilities}
      onAdd={(kind) => {
        onClearError();
        sourceRows.add(kind);
      }}
      onRemove={(key) => {
        onClearError();
        sourceRows.remove(key);
      }}
      onUpdate={(key, patch) => {
        onClearError();
        sourceRows.update(key, patch);
      }}
      repositoryPlacement={placement.eligible ? placement.placement : undefined}
      onRepositoryPlacementChange={placement.setPlacement}
      placementPreview={placement.preview}
      placementPreviewError={placement.previewError}
      placementPreviewLoading={placement.previewLoading}
      isMobile={isMobile}
    />
  );
}

function canSubmitWorkspaceSources(
  rowCount: number,
  placement: ReturnType<typeof useWorkspaceSourcePlacement>,
): boolean {
  const selectedPlacement = placement.preview?.supported_placements.find(
    (option) => option.placement === placement.placement,
  );
  return (
    rowCount > 0 &&
    (!placement.eligible ||
      Boolean(
        placement.placement &&
        placement.preview &&
        !placement.previewError &&
        selectedPlacement?.enabled === true,
      ))
  );
}

function workspaceChangeRestarts(
  placementEligible: boolean,
  executorType: string | null | undefined,
): boolean {
  if (placementEligible) return false;
  if (executorType === "local" || executorType === "local_pc") return false;
  return !isRemoteWorkspaceExecutor(executorType);
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
  return ["local_docker", "remote_docker", "ssh", "sprites", "k8s"].includes(executorType ?? "");
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
