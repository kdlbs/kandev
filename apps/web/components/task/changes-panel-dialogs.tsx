"use client";

import { useState, type RefObject } from "react";
import { IconGitCommit, IconLoader2, IconCheck } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from "@kandev/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Trans, useTranslation } from "react-i18next";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";

// --- Discard Confirmation Dialog ---

type DiscardDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileToDiscard: string | null;
  filesToDiscard: string[] | null;
  anchorRef: RefObject<HTMLElement | null>;
  onConfirm: () => void;
};

function DiscardFileDescription({ file }: { file: string | null }) {
  return (
    <Trans i18nKey="task:discardChangesToFile" values={{ file }}>
      This will permanently discard all changes to{" "}
      <span className="font-semibold [overflow-wrap:anywhere]">{file}</span>. This action cannot be
      undone.
    </Trans>
  );
}

export function DiscardDialog({
  open,
  onOpenChange,
  fileToDiscard,
  filesToDiscard,
  anchorRef,
  onConfirm,
}: DiscardDialogProps) {
  const { t } = useTranslation();
  const bulkCount = filesToDiscard?.length ?? 0;
  const isBulk = bulkCount > 1;
  const displayFile = fileToDiscard ?? (filesToDiscard?.length === 1 ? filesToDiscard[0] : null);
  const description = isBulk ? t("task:discardAllChangesToFiles", { count: bulkCount }) : null;
  const title = t("task:discardChanges");
  const cancelLabel = t("common:cancel");
  const confirmLabel = t("task:discard");

  if (!isBulk) {
    return (
      <ActionConfirmPopover
        open={open}
        anchorRef={anchorRef}
        title={title}
        description={<DiscardFileDescription file={displayFile} />}
        cancelLabel={cancelLabel}
        confirmLabel={confirmLabel}
        confirmTestId="discard-local-changes-confirm"
        testId="discard-local-changes-confirm-popover"
        onOpenChange={onOpenChange}
        onCancel={() => onOpenChange(false)}
        onConfirm={onConfirm}
      />
    );
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription className="break-words">
            {description ?? <DiscardFileDescription file={displayFile} />}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{cancelLabel}</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// --- Amend Commit Message Dialog ---

type AmendDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  amendMessage: string;
  onAmendMessageChange: (value: string) => void;
  onAmend: () => void;
  isLoading: boolean;
};

export function AmendDialog({
  open,
  onOpenChange,
  amendMessage,
  onAmendMessageChange,
  onAmend,
  isLoading,
}: AmendDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitCommit className="h-5 w-5" />
            {t("task:amendCommitMessage")}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <p className="text-sm text-muted-foreground">{t("task:editTheMessageForTheMost")}</p>
          <Textarea
            placeholder={t("task:enterNewCommitMessage")}
            value={amendMessage}
            onChange={(e) => onAmendMessageChange(e.target.value)}
            rows={4}
            className="resize-none"
            autoFocus
          />
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" className="cursor-pointer">
              {t("common:cancel")}
            </Button>
          </DialogClose>
          <Button
            onClick={onAmend}
            disabled={!amendMessage.trim() || isLoading}
            className="cursor-pointer"
          >
            {isLoading ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                {t("task:amending")}
              </>
            ) : (
              <>
                <IconCheck className="h-4 w-4 mr-2" />
                {t("task:amend")}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// --- Reset to Commit Dialog ---

type ResetDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  commitSha: string | null;
  onReset: (mode: "soft" | "hard") => void;
  isLoading: boolean;
};

/** Type-to-confirm field for a hard reset. The SHA is never translated — the
 * user must type it verbatim and it is compared with `===`. */
function HardResetConfirmField({
  shortSha,
  confirmation,
  onConfirmationChange,
}: {
  shortSha: string;
  confirmation: string;
  onConfirmationChange: (value: string) => void;
}) {
  return (
    <div className="space-y-2 p-3 border border-destructive/50 rounded-md bg-destructive/5">
      <p className="text-xs text-destructive font-medium">
        <Trans i18nKey="task:typeShaToConfirm" values={{ shortSha }}>
          Type <code className="bg-muted px-1 rounded">{shortSha}</code> to confirm:
        </Trans>
      </p>
      <Input
        value={confirmation}
        onChange={(e) => onConfirmationChange(e.target.value)}
        placeholder={shortSha}
        className="font-mono h-8 text-sm"
        autoComplete="off"
      />
    </div>
  );
}

export function ResetDialog({
  open,
  onOpenChange,
  commitSha,
  onReset,
  isLoading,
}: ResetDialogProps) {
  const { t } = useTranslation();
  const shortSha = commitSha?.slice(0, 7) ?? "";
  const [mode, setMode] = useState<"soft" | "hard">("soft");
  const [confirmation, setConfirmation] = useState("");

  const isHardResetConfirmed = mode === "hard" && confirmation === shortSha;
  const canReset = !!commitSha && (mode === "soft" || isHardResetConfirmed);

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      // Reset state when closing
      setMode("soft");
      setConfirmation("");
    }
    onOpenChange(newOpen);
  };

  const handleReset = () => {
    if (canReset) {
      onReset(mode);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[450px]">
        <DialogHeader>
          <DialogTitle>{t("task:resetToCommitSha", { shortSha })}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <RadioGroup
            value={mode}
            onValueChange={(v) => setMode(v as "soft" | "hard")}
            className="space-y-3"
          >
            <div className="flex items-start space-x-3">
              <RadioGroupItem value="soft" id="reset-soft" className="mt-1" />
              <div className="flex-1">
                <Label htmlFor="reset-soft" className="font-medium cursor-pointer">
                  {t("task:softReset")}
                </Label>
                <p className="text-sm text-muted-foreground">
                  {t("task:moveHeadToThisCommitKeep")}
                </p>
              </div>
            </div>
            <div className="flex items-start space-x-3">
              <RadioGroupItem value="hard" id="reset-hard" className="mt-1" />
              <div className="flex-1">
                <Label htmlFor="reset-hard" className="font-medium cursor-pointer text-destructive">
                  {t("task:hardReset")}
                </Label>
                <p className="text-sm text-muted-foreground">
                  {t("task:discardAllChangesPermanentlyThisCannot")}
                </p>
              </div>
            </div>
          </RadioGroup>

          {mode === "hard" && (
            <HardResetConfirmField
              shortSha={shortSha}
              confirmation={confirmation}
              onConfirmationChange={setConfirmation}
            />
          )}
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" disabled={isLoading} className="cursor-pointer">
              {t("common:cancel")}
            </Button>
          </DialogClose>
          <Button
            onClick={handleReset}
            disabled={!canReset || isLoading}
            variant={mode === "hard" ? "destructive" : "default"}
            className="cursor-pointer"
          >
            {isLoading ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                {t("common:resetting")}
              </>
            ) : (
              <>{t("common:reset")}</>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
