"use client";

import { useCallback, useEffect, useState } from "react";
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
import { Spinner } from "@kandev/ui/spinner";
import { RemoteRepoChip } from "@/components/task-create-dialog-remote-repo-chip";
import {
  inspectedRemoteRepositoryUpdate,
  remoteRepositoryUpdateNeeded,
} from "@/components/task-create-dialog-remote-repo-identity";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import { useRemoteRepositories } from "@/hooks/domains/integrations/use-remote-repositories";
import { useBranchesByURL } from "@/hooks/domains/github/use-branches-by-url";
import { usePRInfoByURL } from "@/hooks/domains/github/use-pr-info-by-url";
import { registerRemoteRepositoryAction } from "@/app/actions/workspaces";
import type { Repository } from "@/lib/types/http";
import { remoteRepositoryRegistrationPayload } from "./workspace-remote-repository-registration";

const EMPTY_ROW: TaskRemoteRepoRow = { key: "remote", url: "", branch: "", source: "paste" };

type AddRemoteRepositoryDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  /** Called with the saved repository; the page decides how to list it. */
  onRegistered: (repository: Repository) => void;
};

/**
 * Registers a provider-hosted repository from the workspace Repositories page.
 * The form (and every provider request it makes) mounts only while the dialog
 * is open, and the dialog refuses to close while a registration is pending.
 */
export function AddRemoteRepositoryDialog({
  open,
  onOpenChange,
  workspaceId,
  onRegistered,
}: AddRemoteRepositoryDialogProps) {
  const { t } = useTranslation();
  const [submitting, setSubmitting] = useState(false);
  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && submitting) return;
    onOpenChange(nextOpen);
  };
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("workspaces:addRemoteRepository")}</DialogTitle>
          <DialogDescription>{t("workspaces:addRemoteRepositoryDescription")}</DialogDescription>
        </DialogHeader>
        <RemoteRepositoryForm
          workspaceId={workspaceId}
          submitting={submitting}
          setSubmitting={setSubmitting}
          onRegistered={(repository) => {
            onRegistered(repository);
            onOpenChange(false);
          }}
          onCancel={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}

type RemoteRepositoryFormProps = {
  workspaceId: string;
  submitting: boolean;
  setSubmitting: (submitting: boolean) => void;
  onRegistered: (repository: Repository) => void;
  onCancel: () => void;
};

/**
 * The picker, branch, and submit state. It reuses the New Task remote picker,
 * so every provider that lists there (built-in hosts and plugin providers
 * alike) is available here too. A pasted URL is inspected the same way task
 * creation inspects it; a plugin provider's descriptor replaces the bare URL
 * before the row can be submitted.
 */
function RemoteRepositoryForm({
  workspaceId,
  submitting,
  setSubmitting,
  onRegistered,
  onCancel,
}: RemoteRepositoryFormProps) {
  const { t } = useTranslation();
  const [row, setRow] = useState<TaskRemoteRepoRow>(EMPTY_ROW);
  const [error, setError] = useState<string | null>(null);
  const accessibleRepos = useRemoteRepositories(workspaceId);
  const { branches, prInfo, resolutionError, resolving, retryResolution } = useRemoteRowResolution(
    workspaceId,
    row,
    setRow,
  );

  const handleURLChange = useCallback<Parameters<typeof RemoteRepoChip>[0]["onURLChange"]>(
    (url, source, metadata) => {
      setError(null);
      setRow(rowFromURLChange(url, source, metadata));
    },
    [],
  );

  const handleConfirm = async () => {
    if (!row.url.trim() || submitting || resolving || resolutionError) return;
    setSubmitting(true);
    setError(null);
    try {
      const repository = await registerRemoteRepositoryAction(
        workspaceId,
        remoteRepositoryRegistrationPayload(row),
      );
      onRegistered(repository);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("workspaces:couldNotAddRemoteRepository"),
      );
    } finally {
      setSubmitting(false);
    }
  };

  const canSave = Boolean(row.url.trim()) && !submitting && !resolving && !resolutionError;
  return (
    <>
      <div className="space-y-3">
        <RemoteRepoChip
          workspaceId={workspaceId}
          row={row}
          branches={branches.branches(row.url)}
          branchesLoading={branches.loading(row.url)}
          prInfo={prInfo.info(row.url)}
          resolutionError={resolutionError}
          onRetry={retryResolution}
          accessibleRepos={accessibleRepos}
          onURLChange={handleURLChange}
          onBranchChange={(branch) => setRow((current) => ({ ...current, branch }))}
          onRemove={() => setRow(EMPTY_ROW)}
        />
        <RegistrationStatus error={error} submitting={submitting} />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel} disabled={submitting}>
          {t("common:cancel")}
        </Button>
        <Button type="button" onClick={handleConfirm} disabled={!canSave}>
          {t("workspaces:addRepositoryToWorkspace")}
        </Button>
      </DialogFooter>
    </>
  );
}

type URLChange = Parameters<Parameters<typeof RemoteRepoChip>[0]["onURLChange"]>;

/** Picker selections carry their metadata; a pasted URL starts bare. */
function rowFromURLChange(
  url: URLChange[0],
  source: URLChange[1],
  metadata: URLChange[2],
): TaskRemoteRepoRow {
  return {
    key: "remote",
    url,
    source,
    branch: metadata?.defaultBranch ?? "",
    remoteUrl: metadata?.remoteUrl,
    provider: metadata?.provider,
    providerHost: metadata?.providerHost,
    providerScope: metadata?.providerScope,
    providerRepoId: metadata?.providerRepoId,
    providerOwner: metadata?.providerOwner,
    providerName: metadata?.providerName,
    fullName: metadata?.fullName,
  };
}

/**
 * Loads branches and provider inspection for the row's URL. A pasted URL that
 * a registered plugin provider claims gets that provider's descriptor written
 * onto the row; until inspection settles the row counts as resolving.
 */
function useRemoteRowResolution(
  workspaceId: string,
  row: TaskRemoteRepoRow,
  setRow: (row: TaskRemoteRepoRow) => void,
) {
  const branches = useBranchesByURL(workspaceId);
  const prInfo = usePRInfoByURL(workspaceId);
  const { inspection, ensure: ensureInfo } = prInfo;

  useEffect(() => {
    if (!row.url) return;
    branches.ensure(row.url);
    ensureInfo(row.url);
  }, [branches, ensureInfo, row.url]);

  useEffect(() => {
    if (!row.url || row.source !== "paste" || !inspection) return;
    const resolved = inspection(row.url);
    if (!resolved) return;
    const update = inspectedRemoteRepositoryUpdate(resolved, row);
    if (remoteRepositoryUpdateNeeded(row, update)) setRow({ ...row, ...update });
  }, [inspection, row, setRow]);

  const resolutionError = prInfo.error(row.url) ?? branches.error(row.url);
  const resolving = Boolean(row.url) && row.source === "paste" && !prInfo.settled(row.url);
  const retryResolution = () => {
    branches.clear(row.url);
    prInfo.clear(row.url);
    branches.ensure(row.url);
    prInfo.ensure(row.url);
  };
  return { branches, prInfo, resolutionError, resolving, retryResolution };
}

function RegistrationStatus({ error, submitting }: { error: string | null; submitting: boolean }) {
  const { t } = useTranslation();
  return (
    <>
      <p className="text-xs text-muted-foreground">{t("workspaces:remoteRepositoryBranchHint")}</p>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {t("workspaces:couldNotAddRemoteRepositoryWithReason", { message: error })}
        </p>
      ) : null}
      {submitting ? (
        <p role="status" className="flex items-center gap-2 text-xs text-muted-foreground">
          <Spinner className="size-3" aria-hidden="true" />
          {t("workspaces:addingRepository")}
        </p>
      ) : null}
    </>
  );
}
