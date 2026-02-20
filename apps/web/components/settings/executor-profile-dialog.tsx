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
import { Textarea } from "@kandev/ui/textarea";
import { Switch } from "@kandev/ui/switch";
import type { ExecutorProfile } from "@/lib/types/http";
import { createExecutorProfile, updateExecutorProfile } from "@/lib/api/domains/settings-api";

type ExecutorProfileDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  executorId: string;
  profile?: ExecutorProfile | null;
};

export function ExecutorProfileDialog({
  open,
  onOpenChange,
  executorId,
  profile,
}: ExecutorProfileDialogProps) {
  const isEditing = !!profile;
  const [name, setName] = useState(profile?.name ?? "");
  const [isDefault, setIsDefault] = useState(profile?.is_default ?? false);
  const [setupScript, setSetupScript] = useState(profile?.setup_script ?? "");
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    if (!name.trim()) return;
    setSaving(true);
    try {
      if (isEditing && profile) {
        await updateExecutorProfile(executorId, profile.id, {
          name: name.trim(),
          is_default: isDefault,
          setup_script: setupScript,
        });
      } else {
        await createExecutorProfile(executorId, {
          name: name.trim(),
          is_default: isDefault,
          setup_script: setupScript,
        });
      }
      onOpenChange(false);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{isEditing ? "Edit Profile" : "New Profile"}</DialogTitle>
          <DialogDescription>
            Profiles allow different configurations for the same executor.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="profile-name">Name</Label>
            <Input
              id="profile-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Development, CI, Production"
            />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <Label htmlFor="profile-default">Default profile</Label>
              <p className="text-xs text-muted-foreground">
                Used when no profile is explicitly selected.
              </p>
            </div>
            <Switch id="profile-default" checked={isDefault} onCheckedChange={setIsDefault} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="profile-setup">Setup script</Label>
            <Textarea
              id="profile-setup"
              value={setupScript}
              onChange={(e) => setSetupScript(e.target.value)}
              placeholder="#!/bin/bash&#10;# Commands to run when preparing this environment"
              rows={6}
              className="font-mono text-xs"
            />
            <p className="text-xs text-muted-foreground">
              Runs during environment preparation before the agent starts.
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={!name.trim() || saving} className="cursor-pointer">
            {saving && "Saving..."}
            {!saving && (isEditing ? "Save" : "Create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
