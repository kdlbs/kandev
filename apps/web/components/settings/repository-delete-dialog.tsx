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

type DeleteRepositoryDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: () => void;
  activeSessionCount: number;
  deleteLoading: boolean;
};

export function DeleteRepositoryDialog({
  open,
  onOpenChange,
  onDelete,
  activeSessionCount,
  deleteLoading,
}: DeleteRepositoryDialogProps) {
  const { t } = useTranslation();
  const hasActiveSessions = activeSessionCount > 0;
  // The noun and the pronoun used to be inflected here with a ternary, which put
  // the plural rule at the call site — untranslatable in a language with more
  // than two forms. Both now fold into the `_one` / `_other` catalog entries.
  const description = hasActiveSessions
    ? t("workspaces:deleteRepositoryActiveSessions", { count: activeSessionCount })
    : t("workspaces:deleteRepositoryConfirm");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("workspaces:deleteRepositoryTitle")}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            {hasActiveSessions ? t("workspaces:close") : t("common:cancel")}
          </Button>
          {!hasActiveSessions && (
            <Button
              type="button"
              variant="destructive"
              className="cursor-pointer"
              onClick={onDelete}
              disabled={deleteLoading}
            >
              {t("workspaces:deleteRepository")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
