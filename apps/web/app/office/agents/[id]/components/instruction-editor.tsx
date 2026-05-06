"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconDeviceFloppy, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { ScriptEditor } from "@/components/settings/profile-edit/script-editor";
import type { InstructionFile } from "./agent-instructions-tab";
import { createInstructionFileCompletionProvider } from "./instruction-file-completions";

type InstructionEditorProps = {
  file: InstructionFile | null;
  siblingFilenames: string[];
  onSave: (filename: string, content: string) => Promise<void>;
  onDelete: (filename: string) => Promise<void>;
};

export function InstructionEditor({
  file,
  siblingFilenames,
  onSave,
  onDelete,
}: InstructionEditorProps) {
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

  // Exclude the active filename from autocomplete so users don't accidentally
  // self-reference. Recomputed only when the underlying list or active file
  // changes — Monaco re-registers the provider when the factory identity flips.
  const completionFiles = useMemo(
    () => siblingFilenames.filter((name) => name !== file?.filename),
    [siblingFilenames, file?.filename],
  );
  const completionProvider = useCallback(
    (monaco: typeof import("monaco-editor")) =>
      createInstructionFileCompletionProvider(monaco, completionFiles),
    [completionFiles],
  );

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
      <div className="flex-1 min-h-[400px]">
        <ScriptEditor
          value={content}
          onChange={setContent}
          language="markdown"
          height="100%"
          completionProvider={completionProvider}
        />
      </div>
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
