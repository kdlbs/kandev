"use client";

import { useCallback, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";

type SavePresetDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kind: "mr" | "issue";
  customQuery: string;
  projectFilter: string;
  suggestedLabel: string;
  onSave: (label: string) => void;
};

function SavePresetForm({
  kind,
  customQuery,
  projectFilter,
  suggestedLabel,
  onSave,
  onClose,
}: {
  kind: "mr" | "issue";
  customQuery: string;
  projectFilter: string;
  suggestedLabel: string;
  onSave: (label: string) => void;
  onClose: () => void;
}) {
  const [value, setValue] = useState(suggestedLabel);
  const trimmed = value.trim();
  const canSubmit = trimmed.length > 0;

  const handleSubmit = useCallback(() => {
    if (!trimmed) return;
    onSave(trimmed);
    onClose();
  }, [trimmed, onSave, onClose]);

  return (
    <>
      <DialogHeader>
        <DialogTitle>Save GitLab query</DialogTitle>
        <DialogDescription>
          Save this {kind === "mr" ? "merge request" : "issue"} query to the sidebar for quick
          access later.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="gitlab-preset-label" className="text-xs">
            Name
          </Label>
          <Input
            id="gitlab-preset-label"
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onFocus={(e) => e.target.select()}
            placeholder="e.g. Needs my review"
          />
        </div>
        <div className="flex flex-col gap-1.5 text-xs">
          {customQuery && (
            <div className="flex gap-2">
              <span className="text-muted-foreground shrink-0 w-16">Query</span>
              <code className="font-mono text-[11px] bg-muted rounded px-1.5 py-0.5 break-all">
                {customQuery}
              </code>
            </div>
          )}
          {projectFilter && (
            <div className="flex gap-2">
              <span className="text-muted-foreground shrink-0 w-16">Project</span>
              <code className="font-mono text-[11px] bg-muted rounded px-1.5 py-0.5 break-all">
                {projectFilter}
              </code>
            </div>
          )}
        </div>
      </div>
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

export function SavePresetDialog({
  open,
  onOpenChange,
  kind,
  customQuery,
  projectFilter,
  suggestedLabel,
  onSave,
}: SavePresetDialogProps) {
  const handleClose = useCallback(() => onOpenChange(false), [onOpenChange]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {open && (
          <SavePresetForm
            kind={kind}
            customQuery={customQuery}
            projectFilter={projectFilter}
            suggestedLabel={suggestedLabel}
            onSave={onSave}
            onClose={handleClose}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
