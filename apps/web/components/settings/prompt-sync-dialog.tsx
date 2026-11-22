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
import { PromptConflictReviewList } from "@/components/settings/prompt-conflict-review";
import { PromptRemovalReview } from "@/components/settings/prompt-removal-review";
import { useAppStore } from "@/components/state-provider";
import type {
  PromptSyncController,
  PromptSyncFormState,
} from "@/hooks/domains/settings/use-prompt-sync";
import {
  DEFAULT_PROMPTS_PATH,
  intervalInvalid,
  repoTargetMissing,
  type RepoSyncFormState,
} from "@/lib/settings/repo-sync-form";

const ID_PREFIX = "prompt-sync";

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
        directoryPlaceholder={DEFAULT_PROMPTS_PATH}
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
  if (sync.review.active) return t("settings:promptConflictSaveChoices");
  return sync.config ? t("workflows:update") : t("workflows:save");
}

type FooterProps = {
  sync: PromptSyncController;
  removeConfirming: boolean;
  setRemoveConfirming: (confirming: boolean) => void;
  onSave: () => void;
  onRemove: () => Promise<void>;
};

// RemovalReviewFooter goes back to the fields or removes Prompt Sync with the
// version chosen for each listed prompt.
function RemovalReviewFooter({ sync, onRemove }: Pick<FooterProps, "sync" | "onRemove">) {
  const { t } = useTranslation();
  const [removing, setRemoving] = useState(false);
  const remove = async () => {
    setRemoving(true);
    try {
      await onRemove();
    } catch {
      // The controller already reported the failure; the step stays open.
    } finally {
      setRemoving(false);
    }
  };
  return (
    <DialogFooter>
      <Button
        type="button"
        variant="outline"
        onClick={sync.removal.close}
        disabled={removing}
        className="cursor-pointer"
        data-testid="prompt-sync-removal-back"
      >
        {t("common:back")}
      </Button>
      <Button
        type="button"
        variant="destructive"
        onClick={remove}
        disabled={removing}
        className="cursor-pointer"
        data-testid="prompt-sync-removal-confirm"
        data-dialog-default-action
      >
        {t("workflows:remove")}
      </Button>
    </DialogFooter>
  );
}

// PromptSyncDialogFooter shows Remove and Save on the fields step, and Back
// and Save on the conflict review step.
function PromptSyncDialogFooter({
  sync,
  removeConfirming,
  setRemoveConfirming,
  onSave,
  onRemove,
}: FooterProps) {
  const { t } = useTranslation();
  if (sync.removal.active) return <RemovalReviewFooter sync={sync} onRemove={onRemove} />;
  const requestRemoval = async () => {
    if ((await sync.startRemoval()) === "confirm") setRemoveConfirming(true);
  };
  return (
    <DialogFooter>
      {sync.review.active ? (
        <Button
          type="button"
          variant="outline"
          onClick={sync.backToFields}
          disabled={sync.saving}
          className="cursor-pointer"
          data-testid="prompt-sync-review-back"
        >
          {t("common:back")}
        </Button>
      ) : (
        <RepoSyncRemovalActions
          testIdPrefix={ID_PREFIX}
          description={t("settings:promptSyncRemoveConfirm")}
          targetKey={JSON.stringify([sync.config?.workspace_id, sync.config?.path])}
          subject={resolvedTarget(t, sync.config ?? sync.form)}
          hasConfig={!!sync.config}
          confirming={removeConfirming}
          saving={sync.saving}
          onRequest={() => void requestRemoval()}
          onCancel={() => setRemoveConfirming(false)}
          onConfirm={onRemove}
        />
      )}
      <Button
        type="button"
        onClick={onSave}
        disabled={saveDisabled(sync) || removeConfirming}
        className="cursor-pointer"
        data-testid="prompt-sync-save"
        data-dialog-default-action
      >
        {saveButtonLabel(t, sync)}
      </Button>
    </DialogFooter>
  );
}

type DialogStep = "fields" | "conflicts" | "removal";

// Catalog keys, not t() calls, because this is module scope.
const STEP_COPY: Record<DialogStep, { title: string; description: string }> = {
  fields: { title: "settings:promptSyncTitle", description: "settings:promptSyncDescription" },
  conflicts: {
    title: "settings:promptConflictReviewTitle",
    description: "settings:promptConflictReviewDescription",
  },
  removal: {
    title: "settings:promptRemovalReviewTitle",
    description: "settings:promptRemovalReviewDescription",
  },
};

function dialogStep(sync: PromptSyncController): DialogStep {
  if (sync.removal.active) return "removal";
  return sync.review.active ? "conflicts" : "fields";
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
    if (!next) {
      setRemoveConfirming(false);
      sync.backToFields();
      sync.removal.close();
    }
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
  const step = dialogStep(sync);

  return (
    <MobileConfirmationHost open={open}>
      {({ active, contentProps }) => (
        <Dialog open={open} onOpenChange={changeOpen}>
          <DialogContent
            className="sm:max-w-lg max-h-[calc(100dvh-2rem)] flex flex-col overflow-hidden"
            data-testid="prompt-sync-dialog"
            enterConfirms={!removeConfirming && step !== "removal"}
            showCloseButton={!active}
            {...contentProps}
          >
            <MobileConfirmationHostBody>
              <div className="min-h-0 flex-1 space-y-4 overflow-y-auto">
                <DialogHeader>
                  <DialogTitle>{t(STEP_COPY[step].title)}</DialogTitle>
                  <DialogDescription>{t(STEP_COPY[step].description)}</DialogDescription>
                </DialogHeader>
                {step === "fields" && <PromptSyncFields sync={sync} />}
                {step === "conflicts" && <PromptConflictReviewList review={sync.review} />}
                {step === "removal" && <PromptRemovalReview removal={sync.removal} />}
                <PromptSyncDialogFooter
                  sync={sync}
                  removeConfirming={removeConfirming}
                  setRemoveConfirming={setRemoveConfirming}
                  onSave={handleSave}
                  onRemove={handleRemove}
                />
              </div>
            </MobileConfirmationHostBody>
          </DialogContent>
        </Dialog>
      )}
    </MobileConfirmationHost>
  );
}
