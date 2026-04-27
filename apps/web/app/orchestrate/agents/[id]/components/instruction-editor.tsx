"use client";

import { useCallback, useEffect, useState } from "react";
import { IconDeviceFloppy, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@kandev/ui/dialog";
import type { InstructionFile } from "./agent-instructions-tab";

type InstructionEditorProps = {
  file: InstructionFile | null;
  onSave: (filename: string, content: string) => Promise<void>;
  onDelete: (filename: string) => Promise<void>;
};

export function InstructionEditor({ file, onSave, onDelete }: InstructionEditorProps) {
  const [content, setContent] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  useEffect(() => {
    setContent(file?.content ?? "");
  }, [file]);

  const isDirty = file != null && content !== file.content;

  const handleSave = useCallback(async () => {
    if (!file) return;
    setIsSaving(true);
    try {
      await onSave(file.filename, content);
    } finally {
      setIsSaving(false);
    }
  }, [file, content, onSave]);

  const handleDelete = useCallback(async () => {
    if (!file) return;
    await onDelete(file.filename);
    setConfirmDelete(false);
  }, [file, onDelete]);

  if (!file) {
    return (
      <div className="flex-1 border border-border rounded-lg flex items-center justify-center">
        <p className="text-sm text-muted-foreground">Select a file to view or edit.</p>
      </div>
    );
  }

  return (
    <div className="flex-1 border border-border rounded-lg flex flex-col">
      <div className="flex items-center justify-between px-4 py-2 border-b border-border">
        <span className="text-sm font-medium">{file.filename}</span>
        <div className="flex items-center gap-1">
          <Button
            variant="outline"
            size="sm"
            onClick={handleSave}
            disabled={!isDirty || isSaving}
            className="cursor-pointer"
          >
            <IconDeviceFloppy className="h-4 w-4 mr-1" />
            {isSaving ? "Saving..." : "Save"}
          </Button>
          {!file.is_entry && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setConfirmDelete(true)}
              className="cursor-pointer text-destructive hover:text-destructive"
            >
              <IconTrash className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>
      <Textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        className="flex-1 rounded-none rounded-b-lg border-0 resize-none font-mono text-sm min-h-[400px]"
        placeholder="Write instruction content here..."
      />
      <DeleteConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        filename={file.filename}
        onConfirm={handleDelete}
      />
    </div>
  );
}

function DeleteConfirmDialog({
  open,
  onOpenChange,
  filename,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  filename: string;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete {filename}?</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          This will permanently delete this instruction file. This action cannot be undone.
        </p>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button variant="destructive" onClick={onConfirm} className="cursor-pointer">
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
