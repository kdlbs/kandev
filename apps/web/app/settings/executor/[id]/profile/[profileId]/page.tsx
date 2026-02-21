"use client";

import { use, useCallback, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Switch } from "@kandev/ui/switch";
import { Separator } from "@kandev/ui/separator";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import { updateExecutorProfile, deleteExecutorProfile } from "@/lib/api/domains/settings-api";
import { SpritesConnectionCard, SpritesInstancesCard } from "@/components/settings/sprites-settings";
import type { Executor, ExecutorProfile, ProfileEnvVar } from "@/lib/types/http";

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

export default function ProfileDetailPage({
  params,
}: {
  params: Promise<{ id: string; profileId: string }>;
}) {
  const { id: executorId, profileId } = use(params);
  const router = useRouter();
  const executor = useAppStore(
    (state) => state.executors.items.find((e: Executor) => e.id === executorId) ?? null,
  );
  const profile = executor?.profiles?.find((p: ExecutorProfile) => p.id === profileId) ?? null;

  if (!executor || !profile) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-muted-foreground">Profile not found</p>
          <Button className="mt-4 cursor-pointer" onClick={() => router.push(`/settings/executor/${executorId}`)}>
            Back to Executor
          </Button>
        </CardContent>
      </Card>
    );
  }

  return <ProfileEditForm key={profile.id} executor={executor} profile={profile} />;
}

function ProfileEditForm({ executor, profile }: { executor: Executor; profile: ExecutorProfile }) {
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const { items: secrets } = useSecrets();

  const [name, setName] = useState(profile.name);
  const [isDefault, setIsDefault] = useState(profile.is_default);
  const [setupScript, setSetupScript] = useState(profile.setup_script ?? "");
  const [envVarRows, setEnvVarRows] = useState<EnvVarRow[]>(() =>
    envVarsToRows(profile.env_vars),
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const isSprites = executor.type === "sprites";
  const spritesSecretId = useMemo(() => {
    const tokenVar = envVarRows.find((r) => r.key === "SPRITES_API_TOKEN" && r.mode === "secret");
    return tokenVar?.secretId;
  }, [envVarRows]);

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
      const updated = await updateExecutorProfile(executor.id, profile.id, {
        name: name.trim(),
        is_default: isDefault,
        setup_script: setupScript,
        env_vars: envVars,
      });
      // Update store
      setExecutors(
        executors.map((e: Executor) =>
          e.id === executor.id
            ? { ...e, profiles: e.profiles?.map((p) => (p.id === updated.id ? updated : p)) }
            : e,
        ),
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save profile");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      await deleteExecutorProfile(executor.id, profile.id);
      setExecutors(
        executors.map((e: Executor) =>
          e.id === executor.id
            ? { ...e, profiles: e.profiles?.filter((p) => p.id !== profile.id) }
            : e,
        ),
      );
      router.push(`/settings/executor/${executor.id}`);
    } catch {
      setDeleting(false);
      setDeleteDialogOpen(false);
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <h2 className="text-2xl font-bold">{profile.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Profile for {executor.name}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={() => router.push(`/settings/executor/${executor.id}`)}
        >
          Back to Executor
        </Button>
      </div>
      <Separator />

      {/* Profile details */}
      <Card>
        <CardHeader>
          <CardTitle>Profile Details</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="profile-name">Name</Label>
            <Input
              id="profile-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
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
        </CardContent>
      </Card>

      {/* Setup script */}
      <Card>
        <CardHeader>
          <CardTitle>Setup Script</CardTitle>
          <CardDescription>
            Runs inside the execution environment (e.g. Docker container, cloud sandbox) during
            setup, before the agent starts.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Textarea
            value={setupScript}
            onChange={(e) => setSetupScript(e.target.value)}
            placeholder="#!/bin/bash&#10;# Commands to run when preparing this environment"
            rows={8}
            className="font-mono text-xs"
          />
        </CardContent>
      </Card>

      {/* Environment variables */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Environment Variables</CardTitle>
              <CardDescription>
                Injected into the execution environment. Variables can reference secrets for sensitive values.
              </CardDescription>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={addEnvVar}
              className="cursor-pointer"
            >
              <IconPlus className="h-3.5 w-3.5 mr-1" />
              Add
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {envVarRows.length === 0 && (
            <p className="text-sm text-muted-foreground">No environment variables configured.</p>
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
        </CardContent>
      </Card>

      {/* Sprites-specific section */}
      {isSprites && spritesSecretId && (
        <>
          <SpritesConnectionCard secretId={spritesSecretId} />
          <SpritesInstancesCard secretId={spritesSecretId} />
        </>
      )}

      {error && <p className="text-sm text-destructive">{error}</p>}

      <div className="flex items-center justify-between">
        <Button
          variant="destructive"
          size="sm"
          onClick={() => setDeleteDialogOpen(true)}
          className="cursor-pointer"
        >
          <IconTrash className="h-4 w-4 mr-1" />
          Delete Profile
        </Button>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={() => router.push(`/settings/executor/${executor.id}`)}
            className="cursor-pointer"
          >
            Cancel
          </Button>
          <Button
            onClick={handleSave}
            disabled={!name.trim() || saving}
            className="cursor-pointer"
          >
            {saving ? "Saving..." : "Save Changes"}
          </Button>
        </div>
      </div>

      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Profile</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this profile? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting ? "Deleting..." : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
