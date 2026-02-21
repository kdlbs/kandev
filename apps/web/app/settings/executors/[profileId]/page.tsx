"use client";

import { use, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { useAppStore } from "@/components/state-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import {
  updateExecutorProfile,
  deleteExecutorProfile,
  listScriptPlaceholders,
} from "@/lib/api/domains/settings-api";
import type { ScriptPlaceholder } from "@/lib/api/domains/settings-api";
import { EXECUTOR_ICON_MAP, getExecutorLabel } from "@/lib/executor-icons";
import { ProfileDetailsCard } from "@/components/settings/profile-edit/profile-details-card";
import {
  McpPolicyCard,
  validateMcpPolicy,
} from "@/components/settings/profile-edit/mcp-policy-card";
import {
  EnvVarsCard,
  useEnvVarRows,
  rowsToEnvVars,
} from "@/components/settings/profile-edit/env-vars-card";
import { ScriptCard } from "@/components/settings/profile-edit/script-card";
import {
  DockerfileBuildCard,
  DockerContainersCard,
} from "@/components/settings/profile-edit/docker-sections";
import { NetworkPoliciesCard } from "@/components/settings/profile-edit/sprites-sections";
import { SpritesApiKeyCard } from "@/components/settings/profile-edit/sprites-api-key-card";
import { SpritesInstancesCard } from "@/components/settings/sprites-settings";
import type { Executor, ExecutorProfile, ProfileEnvVar } from "@/lib/types/http";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";

const EXECUTORS_ROUTE = "/settings/executors";
const SPRITES_TOKEN_KEY = "SPRITES_API_TOKEN";
const DefaultIcon = EXECUTOR_ICON_MAP.local;

function ExecutorTypeIcon({ type }: { type: string }) {
  const Icon = EXECUTOR_ICON_MAP[type] ?? DefaultIcon;
  return <Icon className="h-5 w-5 text-muted-foreground" />;
}

function useProfileFromStore(profileId: string) {
  const executor = useAppStore(
    (state) =>
      state.executors.items.find((e: Executor) => e.profiles?.some((p) => p.id === profileId)) ??
      null,
  );
  const profile = executor?.profiles?.find((p: ExecutorProfile) => p.id === profileId) ?? null;
  return executor && profile ? { executor, profile } : null;
}

function deriveSpritesSecretId(envVars?: ProfileEnvVar[]): string | null {
  const row = envVars?.find((ev) => ev.key === SPRITES_TOKEN_KEY && ev.secret_id);
  return row?.secret_id ?? null;
}

function parseNetworkPolicyRules(config?: Record<string, string>): NetworkPolicyRule[] {
  const raw = config?.sprites_network_policy_rules;
  if (!raw) return [];
  try {
    return JSON.parse(raw) as NetworkPolicyRule[];
  } catch {
    return [];
  }
}

export default function ProfileEditPage({ params }: { params: Promise<{ profileId: string }> }) {
  const { profileId } = use(params);
  const router = useRouter();
  const result = useProfileFromStore(profileId);

  if (!result) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-muted-foreground">Profile not found</p>
          <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
            Back to Executors
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <ProfileEditForm key={result.profile.id} executor={result.executor} profile={result.profile} />
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

  const save = useCallback(
    async (data: {
      name: string;
      mcp_policy?: string;
      config?: Record<string, string>;
      prepare_script: string;
      cleanup_script: string;
      env_vars: ProfileEnvVar[];
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
    },
    [executor.id, profile.id, executors, setExecutors],
  );

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
      router.push(EXECUTORS_ROUTE);
    } catch {
      setDeleting(false);
      setDeleteDialogOpen(false);
    }
  }, [executor.id, profile.id, executors, setExecutors, router]);

  return { saving, error, deleting, deleteDialogOpen, setDeleteDialogOpen, save, remove };
}

function ProfileHeader({ executor, profileName }: { executor: Executor; profileName: string }) {
  const router = useRouter();
  return (
    <>
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <div className="flex items-center gap-2">
            <ExecutorTypeIcon type={executor.type} />
            <h2 className="text-2xl font-bold">{profileName}</h2>
            <Badge variant="outline" className="text-xs">
              {getExecutorLabel(executor.type)}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {getExecutorDescription(executor.type)}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className="cursor-pointer"
        >
          Back to Executors
        </Button>
      </div>
      <Separator />
    </>
  );
}

function ProfileFormActions({
  saving,
  saveDisabled,
  onSave,
  onDelete,
}: {
  saving: boolean;
  saveDisabled: boolean;
  onSave: () => void;
  onDelete: () => void;
}) {
  const router = useRouter();
  return (
    <div className="flex items-center justify-between">
      <Button variant="destructive" size="sm" onClick={onDelete} className="cursor-pointer">
        <IconTrash className="mr-1 h-4 w-4" />
        Delete Profile
      </Button>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className="cursor-pointer"
        >
          Cancel
        </Button>
        <Button onClick={onSave} disabled={saveDisabled} className="cursor-pointer">
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
          <DialogDescription>Are you sure? This action cannot be undone.</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={onDelete}
            disabled={deleting}
            className="cursor-pointer"
          >
            {deleting ? "Deleting..." : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DockerSections({ profile }: { profile: ExecutorProfile }) {
  const [dockerfile, setDockerfile] = useState(profile.config?.dockerfile ?? "");
  const [imageTag, setImageTag] = useState(profile.config?.image_tag ?? "");

  return (
    <>
      <DockerfileBuildCard
        dockerfile={dockerfile}
        onDockerfileChange={setDockerfile}
        imageTag={imageTag}
        onImageTagChange={setImageTag}
      />
      <DockerContainersCard profileId={profile.id} />
    </>
  );
}

function SpritesSections({
  secretId,
  networkRules,
  onNetworkRulesChange,
}: {
  secretId: string | null;
  networkRules: NetworkPolicyRule[];
  onNetworkRulesChange: (rules: NetworkPolicyRule[]) => void;
}) {
  return (
    <>
      {secretId && <SpritesInstancesCard secretId={secretId} />}
      <NetworkPoliciesCard rules={networkRules} onRulesChange={onNetworkRulesChange} />
    </>
  );
}

function useProfileFormState(executor: Executor, profile: ExecutorProfile) {
  const [name, setName] = useState(profile.name);
  const [mcpPolicy, setMcpPolicy] = useState(profile.mcp_policy ?? "");
  const [prepareScript, setPrepareScript] = useState(profile.prepare_script ?? "");
  const [cleanupScript, setCleanupScript] = useState(profile.cleanup_script ?? "");
  const { envVarRows, addEnvVar, removeEnvVar, updateEnvVar } = useEnvVarRows(profile.env_vars);
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);
  const [spritesSecretId, setSpritesSecretId] = useState<string | null>(() =>
    deriveSpritesSecretId(profile.env_vars),
  );
  const [networkPolicyRules, setNetworkPolicyRules] = useState<NetworkPolicyRule[]>(() =>
    parseNetworkPolicyRules(profile.config),
  );

  const isRemote =
    executor.type === "local_docker" ||
    executor.type === "remote_docker" ||
    executor.type === "sprites";
  const isDocker = executor.type === "local_docker" || executor.type === "remote_docker";
  const isSprites = executor.type === "sprites";
  const mcpPolicyError = useMemo(() => validateMcpPolicy(mcpPolicy), [mcpPolicy]);

  useEffect(() => {
    listScriptPlaceholders()
      .then((res) => setPlaceholders(res.placeholders ?? []))
      .catch(() => {});
  }, []);

  const buildEnvVars = useCallback((): ProfileEnvVar[] => {
    const vars = rowsToEnvVars(envVarRows).filter((ev) => ev.key !== SPRITES_TOKEN_KEY);
    if (isSprites && spritesSecretId) {
      vars.push({ key: SPRITES_TOKEN_KEY, secret_id: spritesSecretId });
    }
    return vars;
  }, [envVarRows, isSprites, spritesSecretId]);

  const prepareDesc = isRemote
    ? "Runs inside the execution environment before the agent starts. Type {{ to see available placeholders."
    : "Runs on the host machine before the agent starts.";

  return {
    name,
    setName,
    mcpPolicy,
    setMcpPolicy,
    prepareScript,
    setPrepareScript,
    cleanupScript,
    setCleanupScript,
    envVarRows,
    addEnvVar,
    removeEnvVar,
    updateEnvVar,
    placeholders,
    spritesSecretId,
    setSpritesSecretId,
    networkPolicyRules,
    setNetworkPolicyRules,
    isRemote,
    isDocker,
    isSprites,
    mcpPolicyError,
    buildEnvVars,
    prepareDesc,
  };
}

function ProfileEditForm({ executor, profile }: { executor: Executor; profile: ExecutorProfile }) {
  const { items: secrets } = useSecrets();
  const persistence = useProfilePersistence(executor, profile);
  const form = useProfileFormState(executor, profile);

  const handleSave = () => {
    if (!form.name.trim() || form.mcpPolicyError) return;
    const config: Record<string, string> = { ...profile.config };
    if (form.isSprites && form.networkPolicyRules.length > 0) {
      config.sprites_network_policy_rules = JSON.stringify(form.networkPolicyRules);
    } else {
      delete config.sprites_network_policy_rules;
    }
    void persistence.save({
      name: form.name.trim(),
      mcp_policy: form.mcpPolicy || undefined,
      config,
      prepare_script: form.prepareScript,
      cleanup_script: form.cleanupScript,
      env_vars: form.buildEnvVars(),
    });
  };

  return (
    <div className="space-y-8">
      <ProfileHeader executor={executor} profileName={profile.name} />
      <ProfileDetailsCard name={form.name} onNameChange={form.setName} />
      {form.isSprites && (
        <SpritesApiKeyCard
          secretId={form.spritesSecretId}
          onSecretIdChange={form.setSpritesSecretId}
          secrets={secrets}
        />
      )}
      {form.isDocker && <DockerSections profile={profile} />}
      {form.isSprites && (
        <SpritesSections
          secretId={form.spritesSecretId}
          networkRules={form.networkPolicyRules}
          onNetworkRulesChange={form.setNetworkPolicyRules}
        />
      )}
      <EnvVarsCard
        rows={form.envVarRows}
        secrets={secrets}
        onAdd={form.addEnvVar}
        onUpdate={form.updateEnvVar}
        onRemove={form.removeEnvVar}
      />
      <ScriptCard
        title="Prepare Script"
        description={form.prepareDesc}
        value={form.prepareScript}
        onChange={form.setPrepareScript}
        height="300px"
        placeholders={form.placeholders}
        executorType={executor.type}
      />
      {form.isRemote && (
        <ScriptCard
          title="Cleanup Script"
          description="Runs after the agent session ends for cleanup tasks."
          value={form.cleanupScript}
          onChange={form.setCleanupScript}
          height="200px"
          placeholders={form.placeholders}
          executorType={executor.type}
        />
      )}
      <McpPolicyCard
        mcpPolicy={form.mcpPolicy}
        mcpPolicyError={form.mcpPolicyError}
        onPolicyChange={form.setMcpPolicy}
      />
      {persistence.error && <p className="text-sm text-destructive">{persistence.error}</p>}
      <ProfileFormActions
        saving={persistence.saving}
        saveDisabled={!form.name.trim() || Boolean(form.mcpPolicyError) || persistence.saving}
        onSave={handleSave}
        onDelete={() => persistence.setDeleteDialogOpen(true)}
      />
      <DeleteProfileDialog
        open={persistence.deleteDialogOpen}
        onOpenChange={persistence.setDeleteDialogOpen}
        onDelete={persistence.remove}
        deleting={persistence.deleting}
      />
    </div>
  );
}

function getExecutorDescription(type: string): string {
  if (type === "local") return "Runs agents directly in the repository folder.";
  if (type === "worktree") return "Creates git worktrees for isolated agent sessions.";
  if (type === "local_docker") return "Runs Docker containers on this machine.";
  if (type === "remote_docker") return "Connects to a remote Docker host.";
  if (type === "sprites") return "Runs agents in Sprites.dev cloud sandboxes.";
  return "Custom executor.";
}
