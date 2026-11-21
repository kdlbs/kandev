"use client";

import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import type { TFunction } from "i18next";
import type {
  WorkflowSyncController,
  WorkflowSyncFormState,
} from "@/hooks/domains/settings/use-workflow-sync";
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";
import {
  useWorkflowSyncLifetime,
  type WorkflowSyncLifetime,
} from "@/hooks/domains/settings/use-workflow-sync-lifetime";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "@/components/confirmation/mobile-confirmation-host";
import {
  DEFAULT_WORKFLOWS_PATH,
  intervalInvalid,
  repoTargetMissing,
} from "@/lib/settings/repo-sync-form";
import {
  BranchDirectoryFields,
  PollFields,
  ProviderField,
  RepoUrlField,
  resolvedTarget,
} from "@/components/settings/repo-sync-fields";
import { RepoSyncRemovalActions } from "@/components/settings/repo-sync-removal-actions";

const WORKFLOW_EXPORT_FORMAT = "kandev_workflow";
const WORKFLOW_EXPORT_EXTENSIONS = ".yml/.yaml/.json";
const ID_PREFIX = "workflow-sync";

function isSaveDisabled(sync: WorkflowSyncController): boolean {
  const targetMissing = repoTargetMissing(sync.form);
  return (
    sync.saving || sync.loading || sync.urlInvalid || intervalInvalid(sync.form) || targetMissing
  );
}

function useClearWorkflowSyncRemovalConfirmation(
  open: boolean,
  sync: WorkflowSyncController,
  setRemoveConfirming: Dispatch<SetStateAction<boolean>>,
) {
  // Clear a pending confirmation when the dialog closes or when its configured
  // target changes. Status-only refreshes leave target fields unchanged, so
  // they do not interrupt an action the user is already confirming.
  useEffect(() => {
    setRemoveConfirming(false);
  }, [
    open,
    sync.config?.workspace_id,
    sync.config?.provider,
    sync.config?.repo_owner,
    sync.config?.repo_name,
    sync.config?.project_path,
    sync.config?.branch,
    sync.config?.path,
    sync.form.provider,
    sync.form.repo_owner,
    sync.form.repo_name,
    sync.form.project_path,
    sync.form.branch,
    sync.form.path,
    setRemoveConfirming,
  ]);
}

type WorkflowSyncDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sync: WorkflowSyncController;
};

function WorkflowSyncFields({ sync }: { sync: WorkflowSyncController }) {
  const { t } = useTranslation();
  const switchingProvider = sync.config && sync.config.provider !== sync.form.provider;
  return (
    <div className="space-y-4">
      <ProviderField provider={sync.form.provider} onChange={sync.setProvider} />
      {switchingProvider && (
        <p className="text-xs text-destructive" data-testid="workflow-sync-provider-switch-warning">
          {t("workflows:switchProviderWarning")}
        </p>
      )}
      <RepoUrlField
        idPrefix={ID_PREFIX}
        provider={sync.form.provider}
        url={sync.url}
        invalid={sync.urlInvalid}
        resolved={resolvedTarget(t, sync.form)}
        onChange={sync.setUrlInput}
      />
      <BranchDirectoryFields
        idPrefix={ID_PREFIX}
        form={sync.form}
        update={sync.update}
        directoryPlaceholder={DEFAULT_WORKFLOWS_PATH}
      />
      <PollFields idPrefix={ID_PREFIX} form={sync.form} update={sync.update} />
      <p className="text-xs text-muted-foreground">
        {t("workflows:syncDirectoryHelp", {
          format: WORKFLOW_EXPORT_FORMAT,
          extensions: WORKFLOW_EXPORT_EXTENSIONS,
        })}
      </p>
    </div>
  );
}

async function saveCurrentDraft(
  sync: WorkflowSyncController,
  completion: WorkflowSyncLifetime,
  close: () => void,
) {
  if (!completion.active) return;
  let draftAccepted = false;
  const saved = await sync.handleSave(() => {
    draftAccepted = true;
  });
  if (completion.active && saved && draftAccepted) close();
}

// WorkflowSyncDialog holds the workflow sync configuration form. It closes
// itself after an adopted save or successful removal. Newer drafts and failures
// keep it open, with request feedback surfaced via toast.
export function WorkflowSyncDialog({ open, onOpenChange, sync }: WorkflowSyncDialogProps) {
  const { t } = useTranslation();
  const [removeConfirming, setRemoveConfirming] = useState(false);
  const hasConfig = !!sync.config;
  const disableSave = isSaveDisabled(sync);
  const targetKey = workflowSyncConfirmationTarget(sync);
  const completion = useWorkflowSyncLifetime(sync.lifetime, open);
  const committedOpenChange = useRef(onOpenChange);
  useLayoutEffect(() => {
    committedOpenChange.current = onOpenChange;
  }, [onOpenChange]);
  const generation = useRef(0);
  useLayoutEffect(() => {
    generation.current += 1;
    return () => {
      generation.current += 1;
    };
  }, [open, targetKey]);

  useClearWorkflowSyncRemovalConfirmation(open, sync, setRemoveConfirming);

  const handleSave = () =>
    saveCurrentDraft(sync, completion, () => committedOpenChange.current(false));
  const handleRemove = async () => {
    if (!completion.active) return;
    const requestGeneration = generation.current;
    let successGeneration = requestGeneration;
    const removed = await sync.handleDelete(() => {
      // Read the target before removal's own config/reset changes it.
      successGeneration = generation.current;
    });
    if (!completion.active) return;
    if (removed) {
      if (successGeneration !== requestGeneration) return;
      committedOpenChange.current(false);
      return;
    }
    if (generation.current !== requestGeneration) return;
    // The confirmation owns retry state; the controller owns failure feedback.
    return Promise.reject();
  };
  const handleDialogOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) setRemoveConfirming(false);
    onOpenChange(nextOpen);
  };

  return (
    <MobileConfirmationHost open={open}>
      {({ active, contentProps }) => (
        <Dialog open={open} onOpenChange={handleDialogOpenChange}>
          <DialogContent
            className="sm:max-w-lg max-h-[calc(100dvh-2rem)] flex flex-col overflow-hidden"
            data-testid="workflow-sync-dialog"
            enterConfirms={!removeConfirming}
            showCloseButton={!active}
            onEscapeKeyDown={(event) => {
              if (!removeConfirming) return;
              event.preventDefault();
              setRemoveConfirming(false);
            }}
            {...contentProps}
          >
            <MobileConfirmationHostBody>
              <div className="min-h-0 flex-1 space-y-4 overflow-y-auto">
                <DialogHeader>
                  <DialogTitle>{t("workflows:syncTitle")}</DialogTitle>
                  <DialogDescription>{t("workflows:syncDescription")}</DialogDescription>
                </DialogHeader>
                <WorkflowSyncFields sync={sync} />
                <DialogFooter>
                  <RepoSyncRemovalActions
                    testIdPrefix={ID_PREFIX}
                    description={t("workflows:removeSyncConfirm")}
                    targetKey={targetKey}
                    subject={resolvedTarget(t, sync.config ?? sync.form)}
                    hasConfig={hasConfig}
                    confirming={removeConfirming}
                    saving={sync.saving}
                    onRequest={() => setRemoveConfirming(true)}
                    onCancel={() => setRemoveConfirming(false)}
                    onConfirm={handleRemove}
                  />
                  <Button
                    type="button"
                    onClick={handleSave}
                    disabled={disableSave || removeConfirming}
                    className="cursor-pointer"
                    data-testid="workflow-sync-save"
                    data-dialog-default-action
                  >
                    {saveLabel(t, sync.saving, hasConfig)}
                  </Button>
                </DialogFooter>
              </div>
            </MobileConfirmationHostBody>
          </DialogContent>
        </Dialog>
      )}
    </MobileConfirmationHost>
  );
}

function workflowSyncConfirmationTarget(sync: WorkflowSyncController) {
  const fields = (config: WorkflowSyncFormState | WorkflowSyncConfig | null) =>
    config && [
      config.provider,
      config.github_host ?? "github.com",
      config.repo_owner,
      config.repo_name,
      config.project_path,
      config.branch,
      config.path,
    ];
  return JSON.stringify([sync.config?.workspace_id, fields(sync.config), fields(sync.form)]);
}

function saveLabel(t: TFunction, saving: boolean, hasConfig: boolean): string {
  if (saving) return t("workflows:saving");
  return hasConfig ? t("workflows:update") : t("workflows:save");
}
