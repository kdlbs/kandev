"use client";

import { useState, useEffect } from "react";
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
import { Button } from "@kandev/ui/button";
import { createExecutorProfile } from "@/lib/api/domains/settings-api";
import type { ExecutorProfile } from "@/lib/types/http";

type ExecutorProfileDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  executorId: string;
  onSaved?: (profile: ExecutorProfile) => void;
};

export function ExecutorProfileDialog({
  open,
  onOpenChange,
  executorId,
  onSaved,
}: ExecutorProfileDialogProps) {
  const [name, setName] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName("");
      setError(null);
    }
  }, [open]);

  const handleSave = async () => {
    if (!name.trim()) return;
    setSaving(true);
    setError(null);
    try {
      const created = await createExecutorProfile(executorId, {
        name: name.trim(),
      });
      onOpenChange(false);
      onSaved?.(created);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create profile");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>New Profile</DialogTitle>
          <DialogDescription>
            Create a new profile for this executor. You can configure scripts and environment
            variables on the profile page.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="profile-name">Name</Label>
            <Input
              id="profile-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Production, Development"
              autoFocus
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={!name.trim() || saving} className="cursor-pointer">
            {saving ? "Creating..." : "Create Profile"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
