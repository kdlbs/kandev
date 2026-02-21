"use client";

import { useCallback, useEffect, useState } from "react";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import type { ExecutorProfile, ProfileEnvVar } from "@/lib/types/http";
import { createExecutorProfile, updateExecutorProfile } from "@/lib/api/domains/settings-api";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";

type EnvVarRow = {
  key: string;
  mode: "value" | "secret";
  value: string;
  secretId: string;
};

function envVarsToRows(envVars?: ProfileEnvVar[]): EnvVarRow[] {
  if (!envVars || envVars.length === 0) return [];
  return envVars.map((ev) => ({
    key: ev.key,
    mode: ev.secret_id ? "secret" : "value",
    value: ev.value ?? "",
    secretId: ev.secret_id ?? "",
  }));
}

function rowsToEnvVars(rows: EnvVarRow[]): ProfileEnvVar[] {
  return rows
    .filter((r) => r.key.trim())
    .map((r) => {
      if (r.mode === "secret" && r.secretId) {
        return { key: r.key.trim(), secret_id: r.secretId };
      }
      return { key: r.key.trim(), value: r.value };
    });
}

type ExecutorProfileDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  executorId: string;
  profile?: ExecutorProfile | null;
  onSaved?: () => void;
};

export function ExecutorProfileDialog({
  open,
  onOpenChange,
  executorId,
  profile,
  onSaved,
}: ExecutorProfileDialogProps) {
  const isEditing = !!profile;
  const [name, setName] = useState(profile?.name ?? "");
  const [isDefault, setIsDefault] = useState(profile?.is_default ?? false);
  const [setupScript, setSetupScript] = useState(profile?.setup_script ?? "");
  const [envVarRows, setEnvVarRows] = useState<EnvVarRow[]>(() =>
    envVarsToRows(profile?.env_vars),
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { items: secrets } = useSecrets();

  // Reset form when profile changes
  useEffect(() => {
    setName(profile?.name ?? "");
    setIsDefault(profile?.is_default ?? false);
    setSetupScript(profile?.setup_script ?? "");
    setEnvVarRows(envVarsToRows(profile?.env_vars));
    setError(null);
  }, [profile]);

  const addEnvVar = useCallback(() => {
    setEnvVarRows((prev) => [...prev, { key: "", mode: "value", value: "", secretId: "" }]);
  }, []);

  const removeEnvVar = useCallback((index: number) => {
    setEnvVarRows((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const updateEnvVar = useCallback((index: number, field: keyof EnvVarRow, val: string) => {
    setEnvVarRows((prev) =>
      prev.map((row, i) => (i === index ? { ...row, [field]: val } : row)),
    );
  }, []);

  const handleSave = async () => {
    if (!name.trim()) return;
    setSaving(true);
    setError(null);
    try {
      const envVars = rowsToEnvVars(envVarRows);
      if (isEditing && profile) {
        await updateExecutorProfile(executorId, profile.id, {
          name: name.trim(),
          is_default: isDefault,
          setup_script: setupScript,
          env_vars: envVars,
        });
      } else {
        await createExecutorProfile(executorId, {
          name: name.trim(),
          is_default: isDefault,
          setup_script: setupScript,
          env_vars: envVars,
        });
      }
      onOpenChange(false);
      onSaved?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save profile");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg max-h-[85vh] overflow-y-auto">
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
              Runs inside the execution environment (e.g. Docker container, cloud sandbox) during
              setup, before the agent starts.
            </p>
          </div>

          {/* Environment Variables */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Environment variables</Label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={addEnvVar}
                className="cursor-pointer h-7 text-xs"
              >
                <IconPlus className="h-3 w-3 mr-1" />
                Add
              </Button>
            </div>
            {envVarRows.length === 0 && (
              <p className="text-xs text-muted-foreground">
                No environment variables configured. Variables are injected into the execution
                environment and can reference secrets.
              </p>
            )}
            {envVarRows.map((row, idx) => (
              <div key={idx} className="flex items-start gap-2">
                <Input
                  value={row.key}
                  onChange={(e) => updateEnvVar(idx, "key", e.target.value)}
                  placeholder="KEY"
                  className="font-mono text-xs flex-[2]"
                />
                <Select
                  value={row.mode}
                  onValueChange={(v) => updateEnvVar(idx, "mode", v)}
                >
                  <SelectTrigger className="w-[100px] text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="value">Value</SelectItem>
                    <SelectItem value="secret">Secret</SelectItem>
                  </SelectContent>
                </Select>
                {row.mode === "value" ? (
                  <Input
                    value={row.value}
                    onChange={(e) => updateEnvVar(idx, "value", e.target.value)}
                    placeholder="value"
                    className="font-mono text-xs flex-[3]"
                  />
                ) : (
                  <Select
                    value={row.secretId}
                    onValueChange={(v) => updateEnvVar(idx, "secretId", v)}
                  >
                    <SelectTrigger className="flex-[3] text-xs">
                      <SelectValue placeholder="Select secret..." />
                    </SelectTrigger>
                    <SelectContent>
                      {secrets.map((s) => (
                        <SelectItem key={s.id} value={s.id}>
                          {s.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => removeEnvVar(idx)}
                  className="cursor-pointer h-9 w-9 shrink-0"
                >
                  <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
                </Button>
              </div>
            ))}
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}
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
