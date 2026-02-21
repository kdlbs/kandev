"use client";

import { use, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { useAppStore } from "@/components/state-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import { createExecutorProfile, fetchDefaultScripts, listScriptPlaceholders } from "@/lib/api/domains/settings-api";
import type { ScriptPlaceholder } from "@/lib/api/domains/settings-api";
import { EXECUTOR_ICON_MAP, getExecutorLabel } from "@/lib/executor-icons";
import { ProfileDetailsCard } from "@/components/settings/profile-edit/profile-details-card";
import { McpPolicyCard, validateMcpPolicy } from "@/components/settings/profile-edit/mcp-policy-card";
import {
  EnvVarsCard,
  useEnvVarRows,
  rowsToEnvVars,
} from "@/components/settings/profile-edit/env-vars-card";
import { ScriptCard } from "@/components/settings/profile-edit/script-card";
import { DockerfileBuildCard } from "@/components/settings/profile-edit/docker-sections";
import { SpritesApiKeyCard } from "@/components/settings/profile-edit/sprites-api-key-card";
import { NetworkPoliciesCard } from "@/components/settings/profile-edit/sprites-sections";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import type { Executor, ProfileEnvVar } from "@/lib/types/http";

const EXECUTORS_ROUTE = "/settings/executors";
const SPRITES_TOKEN_KEY = "SPRITES_API_TOKEN";

const EXECUTOR_TYPE_MAP: Record<string, { executorId: string; label: string; description: string }> = {
  local: { executorId: "exec-local", label: "Local", description: "Runs agents directly in the repository folder." },
  worktree: { executorId: "exec-worktree", label: "Worktree", description: "Creates git worktrees for isolated agent sessions." },
  local_docker: { executorId: "exec-local-docker", label: "Docker", description: "Runs Docker containers on this machine." },
  remote_docker: { executorId: "exec-remote-docker", label: "Remote Docker", description: "Connects to a remote Docker host." },
  sprites: { executorId: "exec-sprites", label: "Sprites.dev", description: "Runs agents in Sprites.dev cloud sandboxes." },
};

const DefaultIcon = EXECUTOR_ICON_MAP.local;

function ExecutorTypeIcon({ type }: { type: string }) {
  const Icon = EXECUTOR_ICON_MAP[type] ?? DefaultIcon;
  return <Icon className="h-5 w-5 text-muted-foreground" />;
}

export default function CreateProfilePage({
  params,
}: {
  params: Promise<{ type: string }>;
}) {
  const { type } = use(params);
  const typeInfo = EXECUTOR_TYPE_MAP[type];

  if (!typeInfo) {
    return <InvalidTypeFallback />;
  }

  return <CreateProfileForm executorType={type} typeInfo={typeInfo} />;
}

function InvalidTypeFallback() {
  const router = useRouter();
  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="text-muted-foreground">Unknown executor type</p>
        <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
          Back to Executors
        </Button>
      </CardContent>
    </Card>
  );
}

function CreateProfileHeader({ type, label, description }: { type: string; label: string; description: string }) {
  const router = useRouter();
  return (
    <>
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <div className="flex items-center gap-2">
            <ExecutorTypeIcon type={type} />
            <h2 className="text-2xl font-bold">New {label} Profile</h2>
            <Badge variant="outline" className="text-xs">{getExecutorLabel(type)}</Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => router.push(EXECUTORS_ROUTE)} className="cursor-pointer">
          Back to Executors
        </Button>
      </div>
      <Separator />
    </>
  );
}

function CreateFormActions({
  saving,
  saveDisabled,
  onSave,
}: {
  saving: boolean;
  saveDisabled: boolean;
  onSave: () => void;
}) {
  const router = useRouter();
  return (
    <div className="flex items-center justify-end gap-2">
      <Button variant="outline" onClick={() => router.push(EXECUTORS_ROUTE)} className="cursor-pointer">
        Cancel
      </Button>
      <Button onClick={onSave} disabled={saveDisabled} className="cursor-pointer">
        {saving ? "Creating..." : "Create Profile"}
      </Button>
    </div>
  );
}

function buildProfileConfig(isSprites: boolean, networkPolicyRules: NetworkPolicyRule[]): Record<string, string> | undefined {
  if (!isSprites || networkPolicyRules.length === 0) return undefined;
  return { sprites_network_policy_rules: JSON.stringify(networkPolicyRules) };
}

function useDefaultScripts(executorType: string, setPrepareScript: (v: string) => void) {
  useEffect(() => {
    fetchDefaultScripts(executorType)
      .then((res) => {
        if (res.prepare_script) setPrepareScript(res.prepare_script);
      })
      .catch(() => {});
  }, [executorType, setPrepareScript]);
}

function CreateProfileForm({
  executorType,
  typeInfo,
}: {
  executorType: string;
  typeInfo: { executorId: string; label: string; description: string };
}) {
  const router = useRouter();
  const { items: secrets } = useSecrets();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);

  const [name, setName] = useState("");
  const [mcpPolicy, setMcpPolicy] = useState("");
  const [prepareScript, setPrepareScript] = useState("");
  const [cleanupScript, setCleanupScript] = useState("");
  const { envVarRows, addEnvVar, removeEnvVar, updateEnvVar } = useEnvVarRows([]);
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [spritesSecretId, setSpritesSecretId] = useState<string | null>(null);
  const [networkPolicyRules, setNetworkPolicyRules] = useState<NetworkPolicyRule[]>([]);

  const isRemote = executorType === "local_docker" || executorType === "remote_docker" || executorType === "sprites";
  const isDocker = executorType === "local_docker" || executorType === "remote_docker";
  const isSprites = executorType === "sprites";
  const mcpPolicyError = useMemo(() => validateMcpPolicy(mcpPolicy), [mcpPolicy]);

  const [dockerfile, setDockerfile] = useState("");
  const [imageTag, setImageTag] = useState("");

  useEffect(() => {
    listScriptPlaceholders().then((res) => setPlaceholders(res.placeholders ?? [])).catch(() => {});
  }, []);

  useDefaultScripts(executorType, setPrepareScript);

  const buildEnvVars = useCallback((): ProfileEnvVar[] => {
    const vars = rowsToEnvVars(envVarRows).filter((ev) => ev.key !== SPRITES_TOKEN_KEY);
    if (isSprites && spritesSecretId) {
      vars.push({ key: SPRITES_TOKEN_KEY, secret_id: spritesSecretId });
    }
    return vars;
  }, [envVarRows, isSprites, spritesSecretId]);

  const handleSave = useCallback(async () => {
    if (!name.trim() || mcpPolicyError) return;
    setSaving(true);
    setError(null);
    try {
      const profile = await createExecutorProfile(typeInfo.executorId, {
        name: name.trim(),
        mcp_policy: mcpPolicy || undefined,
        config: buildProfileConfig(isSprites, networkPolicyRules),
        prepare_script: prepareScript,
        cleanup_script: cleanupScript,
        env_vars: buildEnvVars(),
      });
      setExecutors(
        executors.map((e: Executor) =>
          e.id === typeInfo.executorId
            ? { ...e, profiles: [...(e.profiles ?? []), profile] }
            : e,
        ),
      );
      router.push(`/settings/executors/${profile.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create profile");
      setSaving(false);
    }
  }, [name, mcpPolicy, mcpPolicyError, prepareScript, cleanupScript, buildEnvVars, isSprites, networkPolicyRules, typeInfo.executorId, executors, setExecutors, router]);

  const prepareDesc = isRemote
    ? "Runs inside the execution environment before the agent starts. Type {{ to see available placeholders."
    : "Runs on the host machine before the agent starts.";

  return (
    <div className="space-y-8">
      <CreateProfileHeader type={executorType} label={typeInfo.label} description={typeInfo.description} />
      <ProfileDetailsCard name={name} onNameChange={setName} />
      {isSprites && (
        <SpritesApiKeyCard secretId={spritesSecretId} onSecretIdChange={setSpritesSecretId} secrets={secrets} />
      )}
      {isDocker && (
        <DockerfileBuildCard
          dockerfile={dockerfile}
          onDockerfileChange={setDockerfile}
          imageTag={imageTag}
          onImageTagChange={setImageTag}
        />
      )}
      {isSprites && (
        <NetworkPoliciesCard rules={networkPolicyRules} onRulesChange={setNetworkPolicyRules} />
      )}
      <EnvVarsCard rows={envVarRows} secrets={secrets} onAdd={addEnvVar} onUpdate={updateEnvVar} onRemove={removeEnvVar} />
      <ScriptCard title="Prepare Script" description={prepareDesc} value={prepareScript} onChange={setPrepareScript} height="300px" placeholders={placeholders} executorType={executorType} />
      {isRemote && (
        <ScriptCard title="Cleanup Script" description="Runs after the agent session ends for cleanup tasks." value={cleanupScript} onChange={setCleanupScript} height="200px" placeholders={placeholders} executorType={executorType} />
      )}
      <McpPolicyCard mcpPolicy={mcpPolicy} mcpPolicyError={mcpPolicyError} onPolicyChange={setMcpPolicy} />
      {error && <p className="text-sm text-destructive">{error}</p>}
      <CreateFormActions
        saving={saving}
        saveDisabled={!name.trim() || Boolean(mcpPolicyError) || saving}
        onSave={handleSave}
      />
    </div>
  );
}
