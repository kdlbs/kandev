"use client";

import { useCallback, useRef, useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";

type TaskRenameDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentTitle: string;
  onSubmit: (newTitle: string) => void;
};

export function TaskRenameDialog({
  open,
  onOpenChange,
  currentTitle,
  onSubmit,
}: TaskRenameDialogProps) {
  const [value, setValue] = useState(currentTitle);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setValue(currentTitle);
      requestAnimationFrame(() => inputRef.current?.select());
    }
  }, [open, currentTitle]);

  const trimmed = value.trim();
  const canSubmit = trimmed.length > 0 && trimmed !== currentTitle;

  const handleSubmit = useCallback(() => {
    if (!canSubmit) return;
    onSubmit(trimmed);
    onOpenChange(false);
  }, [canSubmit, trimmed, onSubmit, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Rename task</DialogTitle>
        </DialogHeader>
        <Input
          ref={inputRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") handleSubmit();
          }}
          placeholder="Task title"
        />
        <DialogFooter>
          <Button
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            className="cursor-pointer"
            disabled={!canSubmit}
            onClick={handleSubmit}
          >
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
