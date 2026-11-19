"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "@/components/confirmation/mobile-confirmation-host";
import {
  BranchDirectoryFields,
  PollFields,
  ProviderField,
  RepoUrlField,
  resolvedTarget,
} from "@/components/settings/repo-sync-fields";
import { RepoSyncRemovalActions } from "@/components/settings/repo-sync-removal-actions";
import { useAppStore } from "@/components/state-provider";
import type {
  PromptSyncController,
  PromptSyncFormState,
} from "@/hooks/domains/settings/use-prompt-sync";
import {
  intervalInvalid,
  repoTargetMissing,
  type RepoSyncFormState,
} from "@/lib/settings/repo-sync-form";

const ID_PREFIX = "prompt-sync";
const DIRECTORY_PLACEHOLDER = ".kandev/prompts";

function WorkspaceField({ sync }: { sync: PromptSyncController }) {
  const { t } = useTranslation();
  const workspaces = useAppStore((state) => state.workspaces.items);
  return (
    <div className="space-y-1.5">
      <Label htmlFor="prompt-sync-workspace">{t("settings:promptSyncWorkspaceLabel")}</Label>
      <Select
        value={sync.form.workspace_id || undefined}
        onValueChange={(value) => sync.update("workspace_id", value)}
      >
        <SelectTrigger
          id="prompt-sync-workspace"
          className="w-full cursor-pointer"
          data-testid="prompt-sync-workspace-select"
        >
          <SelectValue placeholder={t("settings:promptSyncWorkspacePlaceholder")} />
        </SelectTrigger>
        <SelectContent>
          {workspaces.map((workspace) => (
            <SelectItem key={workspace.id} value={workspace.id} className="cursor-pointer">
              {workspace.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("settings:promptSyncWorkspaceHint")}</p>
    </div>
  );
}

function PromptSyncFields({ sync }: { sync: PromptSyncController }) {
  const { t } = useTranslation();
  // Repository fields edit only keys shared with RepoSyncFormState, whose
  // value types are identical in PromptSyncFormState.
  const updateRepo = <K extends keyof RepoSyncFormState>(key: K, value: RepoSyncFormState[K]) =>
    sync.update(key, value as PromptSyncFormState[K]);
  return (
    <div className="space-y-4">
      <ProviderField provider={sync.form.provider} onChange={sync.setProvider} />
      <WorkspaceField sync={sync} />
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
        update={updateRepo}
        directoryPlaceholder={DIRECTORY_PLACEHOLDER}
      />
      <PollFields idPrefix={ID_PREFIX} form={sync.form} update={updateRepo} />
      <p className="text-xs text-muted-foreground">{t("settings:promptSyncDirectoryHelp")}</p>
    </div>
  );
}

function saveDisabled(sync: PromptSyncController): boolean {
  return (
    sync.saving ||
    sync.loading ||
    sync.urlInvalid ||
    !sync.form.workspace_id ||
    intervalInvalid(sync.form) ||
    repoTargetMissing(sync.form)
  );
}

function saveButtonLabel(t: TFunction, sync: PromptSyncController): string {
  if (sync.saving) return t("workflows:saving");
  return sync.config ? t("workflows:update") : t("workflows:save");
}

type PromptSyncDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sync: PromptSyncController;
};

// PromptSyncDialog edits the install-wide Prompt Sync source. It closes after a
// successful save or removal; failures keep it open with toast feedback.
export function PromptSyncDialog({ open, onOpenChange, sync }: PromptSyncDialogProps) {
  const { t } = useTranslation();
  const [removeConfirming, setRemoveConfirming] = useState(false);
  const changeOpen = (next: boolean) => {
    if (!next) setRemoveConfirming(false);
    onOpenChange(next);
  };
  const handleSave = async () => {
    if (await sync.handleSave()) changeOpen(false);
  };
  const handleRemove = async () => {
    if (await sync.handleDelete()) {
      changeOpen(false);
      return;
    }
    // The confirmation owns retry state; the controller owns failure feedback.
    return Promise.reject();
  };
  const saveLabel = saveButtonLabel(t, sync);

  return (
    <MobileConfirmationHost open={open}>
      {({ active, contentProps }) => (
        <Dialog open={open} onOpenChange={changeOpen}>
          <DialogContent
            className="sm:max-w-lg max-h-[calc(100dvh-2rem)] flex flex-col overflow-hidden"
            data-testid="prompt-sync-dialog"
            enterConfirms={!removeConfirming}
            showCloseButton={!active}
            {...contentProps}
          >
            <MobileConfirmationHostBody>
              <div className="min-h-0 flex-1 space-y-4 overflow-y-auto">
                <DialogHeader>
                  <DialogTitle>{t("settings:promptSyncTitle")}</DialogTitle>
                  <DialogDescription>{t("settings:promptSyncDescription")}</DialogDescription>
                </DialogHeader>
                <PromptSyncFields sync={sync} />
                <DialogFooter>
                  <RepoSyncRemovalActions
                    testIdPrefix={ID_PREFIX}
                    description={t("settings:promptSyncRemoveConfirm")}
                    targetKey={JSON.stringify([sync.config?.workspace_id, sync.config?.path])}
                    subject={resolvedTarget(t, sync.config ?? sync.form)}
                    hasConfig={!!sync.config}
                    confirming={removeConfirming}
                    saving={sync.saving}
                    onRequest={() => setRemoveConfirming(true)}
                    onCancel={() => setRemoveConfirming(false)}
                    onConfirm={handleRemove}
                  />
                  <Button
                    type="button"
                    onClick={handleSave}
                    disabled={saveDisabled(sync) || removeConfirming}
                    className="cursor-pointer"
                    data-testid="prompt-sync-save"
                    data-dialog-default-action
                  >
                    {saveLabel}
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
