"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { AzureDevOpsPresetKind } from "./azure-devops-presets";

function SaveViewForm({
  kind,
  onSave,
  onClose,
}: {
  kind: AzureDevOpsPresetKind;
  onSave: (label: string) => Promise<void>;
  onClose: () => void;
}) {
  const [label, setLabel] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const trimmed = label.trim();
  const handleSave = async () => {
    setSaving(true);
    setError("");
    try {
      await onSave(trimmed);
      onClose();
    } catch {
      setError("Could not save this view. Try again.");
    } finally {
      setSaving(false);
    }
  };
  return (
    <>
      <DialogHeader>
        <DialogTitle>Save Azure DevOps view</DialogTitle>
        <DialogDescription>
          Save the current {kind === "work_item" ? "work-item query" : "pull-request filters"} for
          this workspace.
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-1.5">
        <Label htmlFor="azure-devops-view-name">Name</Label>
        <Input
          id="azure-devops-view-name"
          autoFocus
          value={label}
          onChange={(event) => {
            setLabel(event.target.value);
            setError("");
          }}
          placeholder="e.g. Platform triage"
        />
        {error && (
          <p role="alert" className="text-xs text-destructive">
            {error}
          </p>
        )}
      </div>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer"
          disabled={saving}
          onClick={onClose}
        >
          Cancel
        </Button>
        <Button
          type="button"
          className="cursor-pointer"
          disabled={!trimmed || saving}
          onClick={() => void handleSave()}
        >
          {saving ? "Saving..." : "Save"}
        </Button>
      </DialogFooter>
    </>
  );
}

export function AzureDevOpsSaveViewDialog({
  open,
  kind,
  onOpenChange,
  onSave,
}: {
  open: boolean;
  kind: AzureDevOpsPresetKind;
  onOpenChange: (open: boolean) => void;
  onSave: (label: string) => Promise<void>;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {open && <SaveViewForm kind={kind} onSave={onSave} onClose={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  );
}
