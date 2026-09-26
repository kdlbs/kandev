"use client";

import type React from "react";
import { useCallback, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import { useTaskTitleSelectionRestore } from "@/hooks/use-task-title-selection-restore";
import { useTranslation } from "react-i18next";

type TaskRenameDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentTitle: string;
  onSubmit: (newTitle: string) => void;
};

function TaskRenameForm({
  currentTitle,
  onSubmit,
  onClose,
}: {
  currentTitle: string;
  onSubmit: (newTitle: string) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState(currentTitle);
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(value);

  const trimmed = value.trim();
  const canSubmit = trimmed.length > 0 && trimmed !== currentTitle;

  const handleSubmit = useCallback(() => {
    if (!canSubmit) return;
    onSubmit(trimmed);
    onClose();
  }, [canSubmit, trimmed, onSubmit, onClose]);

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("task:renameTask")}</DialogTitle>
      </DialogHeader>
      <Input
        ref={inputRef}
        autoFocus
        value={value}
        onChange={(e) => setValue(clampChange(e))}
        onFocus={(e) => e.target.select()}
        placeholder={t("task:taskTitle")}
      />
      <DialogFooter>
        <Button variant="outline" className="cursor-pointer" onClick={onClose}>
          {t("common:cancel")}
        </Button>
        <Button className="cursor-pointer" disabled={!canSubmit} onClick={handleSubmit}>
          {t("common:save")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function TaskRenameDialog({
  open,
  onOpenChange,
  currentTitle,
  onSubmit,
}: TaskRenameDialogProps): React.JSX.Element {
  const handleClose = useCallback(() => onOpenChange(false), [onOpenChange]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {open && (
          <TaskRenameForm currentTitle={currentTitle} onSubmit={onSubmit} onClose={handleClose} />
        )}
      </DialogContent>
    </Dialog>
  );
}
