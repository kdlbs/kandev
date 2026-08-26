"use client";

import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { useTranslation } from "react-i18next";
import type { RemoteContributionResolutionAction } from "./use-remote-contribution-resolution";

export type RemoteContributionResolutionDialogProps = {
  open: boolean;
  action: RemoteContributionResolutionAction;
  repositoryName: string;
  expectedRemoteHead: string;
  isLoading: boolean;
  errorKey?: string | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

export function RemoteContributionResolutionDialog({
  open,
  action,
  repositoryName,
  expectedRemoteHead,
  isLoading,
  errorKey,
  onOpenChange,
  onConfirm,
}: RemoteContributionResolutionDialogProps) {
  const { t } = useTranslation();
  const isReplace = action === "replace";
  const title = t(isReplace ? "task:replacePRBranch" : "task:usePRVersion");
  const description = isReplace
    ? t("task:replacePRBranchConfirmation", {
        repository: repositoryName,
        head: expectedRemoteHead,
      })
    : t("task:usePRVersionConfirmation", { head: expectedRemoteHead });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="remote-contribution-resolution-dialog" className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {errorKey && <p className="text-sm text-destructive">{t(errorKey)}</p>}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="h-11 md:h-9"
            onClick={() => onOpenChange(false)}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            className="h-11 md:h-9"
            data-testid="remote-contribution-confirm"
            disabled={isLoading}
            onClick={onConfirm}
          >
            {title}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
