"use client";

import { use, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
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
import {
  updateExecutorProfile,
  deleteExecutorProfile,
  listScriptPlaceholders,
} from "@/lib/api/domains/settings-api";
import type { ScriptPlaceholder } from "@/lib/api/domains/settings-api";
import { SpritesConnectionCard, SpritesInstancesCard } from "@/components/settings/sprites-settings";
import { ScriptEditor } from "@/components/settings/profile-edit/script-editor";
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

function ProfileDetailsCard({
  name,
  onNameChange,
}: {
  name: string;
  onNameChange: (v: string) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Profile Details</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="profile-name">Name</Label>
          <Input id="profile-name" value={name} onChange={(e) => onNameChange(e.target.value)} />
        </div>
      </CardContent>
    </Card>
  );
}

function EnvVarRow({
  row,
  index,
  secrets,
  onUpdate,
  onRemove,
}: {
  row: EnvVarRow;
  index: number;
  secrets: { id: string; name: string }[];
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <div className="flex items-start gap-2">
      <Input
        value={row.key}
        onChange={(e) => onUpdate(index, "key", e.target.value)}
        placeholder="KEY"
        className="font-mono text-xs flex-[2]"
      />
      <Select value={row.mode} onValueChange={(v) => onUpdate(index, "mode", v)}>
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
          onChange={(e) => onUpdate(index, "value", e.target.value)}
          placeholder="value"
          className="font-mono text-xs flex-[3]"
        />
      ) : (
        <Select value={row.secretId} onValueChange={(v) => onUpdate(index, "secretId", v)}>
          <SelectTrigger className="flex-[3] text-xs">
            <SelectValue placeholder="Select secret..." />
          </SelectTrigger>
          <SelectContent>
            {secrets.map((s) => (
              <SelectItem key={s.id} value={s.id}>{s.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={() => onRemove(index)}
        className="cursor-pointer h-9 w-9 shrink-0"
      >
        <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
      </Button>
    </div>
  );
}

function EnvVarsCard({
  rows,
  secrets,
  onAdd,
  onUpdate,
  onRemove,
}: {
  rows: EnvVarRow[];
  secrets: { id: string; name: string }[];
  onAdd: () => void;
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Environment Variables</CardTitle>
            <CardDescription>
              Injected into the execution environment. Variables can reference secrets for sensitive values.
            </CardDescription>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={onAdd} className="cursor-pointer">
            <IconPlus className="h-3.5 w-3.5 mr-1" />
            Add
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {rows.length === 0 && (
          <p className="text-sm text-muted-foreground">No environment variables configured.</p>
        )}
        {rows.map((row, idx) => (
          <EnvVarRow key={idx} row={row} index={idx} secrets={secrets} onUpdate={onUpdate} onRemove={onRemove} />
        ))}
      </CardContent>
    </Card>
  );
}

function ScriptCard({
  title,
  description,
  value,
  onChange,
  height,
  placeholders,
  executorType,
}: {
  title: string;
  description: string;
  value: string;
  onChange: (v: string) => void;
  height: string;
  placeholders: ScriptPlaceholder[];
  executorType: string;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="border rounded-md overflow-hidden">
          <ScriptEditor value={value} onChange={onChange} height={height} placeholders={placeholders} executorType={executorType} />
        </div>
      </CardContent>
    </Card>
  );
}

function ProfileActions({
  executorId,
  saving,
  nameValid,
  onSave,
  onRequestDelete,
}: {
  executorId: string;
  saving: boolean;
  nameValid: boolean;
  onSave: () => void;
  onRequestDelete: () => void;
}) {
  const router = useRouter();
  return (
    <div className="flex items-center justify-between">
      <Button variant="destructive" size="sm" onClick={onRequestDelete} className="cursor-pointer">
        <IconTrash className="h-4 w-4 mr-1" />
        Delete Profile
      </Button>
      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={() => router.push(`/settings/executor/${executorId}`)} className="cursor-pointer">
          Cancel
        </Button>
        <Button onClick={onSave} disabled={!nameValid || saving} className="cursor-pointer">
          {saving ? "Saving..." : "Save Changes"}
        </Button>
      </div>
    </div>
  );
}

function DeleteProfileDialog({
  open,
  onOpenChange,
  onDelete,
  deleting,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDelete: () => void;
  deleting: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Profile</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete this profile? This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button variant="destructive" onClick={onDelete} disabled={deleting}>
            {deleting ? "Deleting..." : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function useProfilePersistence(executor: Executor, profile: ExecutorProfile) {
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const save = useCallback(async (data: {
    name: string; prepare_script: string;
    cleanup_script: string; env_vars: ProfileEnvVar[];
  }) => {
    setSaving(true);
    setError(null);
    try {
      const updated = await updateExecutorProfile(executor.id, profile.id, data);
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
  }, [executor.id, profile.id, executors, setExecutors]);

  const remove = useCallback(async () => {
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
  }, [executor.id, profile.id, executors, setExecutors, router]);

  return { saving, error, deleting, deleteDialogOpen, setDeleteDialogOpen, save, remove };
}

function ProfileEditForm({ executor, profile }: { executor: Executor; profile: ExecutorProfile }) {
  const router = useRouter();
  const { items: secrets } = useSecrets();
  const persistence = useProfilePersistence(executor, profile);

  const [name, setName] = useState(profile.name);
  const [prepareScript, setPrepareScript] = useState(profile.prepare_script ?? "");
  const [cleanupScript, setCleanupScript] = useState(profile.cleanup_script ?? "");
  const [envVarRows, setEnvVarRows] = useState<EnvVarRow[]>(() => envVarsToRows(profile.env_vars));
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);

  const isRemote = executor.type === "sprites" || executor.type === "local_docker" || executor.type === "remote_docker";
  const isSprites = executor.type === "sprites";

  const spritesSecretId = useMemo(() => {
    const tokenVar = envVarRows.find((r) => r.key === "SPRITES_API_TOKEN" && r.mode === "secret");
    return tokenVar?.secretId;
  }, [envVarRows]);

  useEffect(() => {
    listScriptPlaceholders().then((res) => setPlaceholders(res.placeholders ?? [])).catch(() => {});
  }, []);

  const addEnvVar = useCallback(() => {
    setEnvVarRows((prev) => [...prev, { key: "", mode: "value", value: "", secretId: "" }]);
  }, []);
  const removeEnvVar = useCallback((index: number) => {
    setEnvVarRows((prev) => prev.filter((_, i) => i !== index));
  }, []);
  const updateEnvVar = useCallback((index: number, field: keyof EnvVarRow, val: string) => {
    setEnvVarRows((prev) => prev.map((row, i) => (i === index ? { ...row, [field]: val } : row)));
  }, []);

  const handleSave = () => {
    if (!name.trim()) return;
    void persistence.save({
      name: name.trim(),
      prepare_script: prepareScript, cleanup_script: cleanupScript,
      env_vars: rowsToEnvVars(envVarRows),
    });
  };

  const prepareDesc = isRemote
    ? "Runs inside the execution environment before the agent starts. Type {{ to see available placeholders."
    : "Runs on the host machine before the agent starts.";

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <h2 className="text-2xl font-bold">{profile.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">Profile for {executor.name}</p>
        </div>
        <Button variant="outline" size="sm" className="cursor-pointer" onClick={() => router.push(`/settings/executor/${executor.id}`)}>
          Back to Executor
        </Button>
      </div>
      <Separator />
      <ProfileDetailsCard name={name} onNameChange={setName} />
      {isSprites && spritesSecretId && (
        <>
          <SpritesConnectionCard secretId={spritesSecretId} />
          <SpritesInstancesCard secretId={spritesSecretId} />
        </>
      )}
      <EnvVarsCard rows={envVarRows} secrets={secrets} onAdd={addEnvVar} onUpdate={updateEnvVar} onRemove={removeEnvVar} />
      <ScriptCard title="Prepare Script" description={prepareDesc} value={prepareScript} onChange={setPrepareScript} height="300px" placeholders={placeholders} executorType={executor.type} />
      {isRemote && (
        <ScriptCard title="Cleanup Script" description="Runs after the agent session ends for cleanup tasks." value={cleanupScript} onChange={setCleanupScript} height="200px" placeholders={placeholders} executorType={executor.type} />
      )}
      {persistence.error && <p className="text-sm text-destructive">{persistence.error}</p>}
      <ProfileActions executorId={executor.id} saving={persistence.saving} nameValid={Boolean(name.trim())} onSave={handleSave} onRequestDelete={() => persistence.setDeleteDialogOpen(true)} />
      <DeleteProfileDialog open={persistence.deleteDialogOpen} onOpenChange={persistence.setDeleteDialogOpen} onDelete={persistence.remove} deleting={persistence.deleting} />
    </div>
  );
}
