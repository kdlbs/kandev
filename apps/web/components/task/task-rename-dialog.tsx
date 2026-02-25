"use client";

import { useCallback, useRef, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";

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
  const [value, setValue] = useState(currentTitle);
  const inputRef = useRef<HTMLInputElement>(null);

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
        <DialogTitle>Rename task</DialogTitle>
      </DialogHeader>
      <Input
        ref={inputRef}
        autoFocus
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") handleSubmit();
        }}
        onFocus={(e) => e.target.select()}
        placeholder="Task title"
      />
      <DialogFooter>
        <Button variant="outline" className="cursor-pointer" onClick={onClose}>
          Cancel
        </Button>
        <Button className="cursor-pointer" disabled={!canSubmit} onClick={handleSubmit}>
          Save
        </Button>
      </DialogFooter>
    </>
  );
}

export function TaskRenameDialog({ open, onOpenChange, currentTitle, onSubmit }: TaskRenameDialogProps) {
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
