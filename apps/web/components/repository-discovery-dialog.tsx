"use client";

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
import { RepositoryDiscoveryControls } from "@/components/repository-discovery-controls";

export type RepositoryDiscoveryDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  isInitialLoading?: boolean;
};

export function RepositoryDiscoveryDialog({
  open,
  onOpenChange,
  workspaceId,
  isInitialLoading = false,
}: RepositoryDiscoveryDialogProps) {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl" data-testid="repository-discovery-dialog">
        <DialogHeader>
          <DialogTitle>{t("workspaces:repositoryDiscoveryTitle")}</DialogTitle>
          <DialogDescription>
            {t("workspaces:chooseFoldersToDiscoverRepositoriesDescription")}
          </DialogDescription>
        </DialogHeader>
        <div className="py-2">
          <RepositoryDiscoveryControls
            workspaceId={workspaceId}
            enabled={open}
            presentation="dialog"
            isInitialLoading={isInitialLoading}
          />
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            {t("common:close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
