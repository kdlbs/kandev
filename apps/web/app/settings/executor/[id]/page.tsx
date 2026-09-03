"use client";

import { useMemo, useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Textarea } from "@kandev/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { updateExecutorAction, deleteExecutorAction } from "@/app/actions/executors";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore } from "@/components/state-provider";
import { ExecutorProfilesCard } from "@/components/settings/executor-profiles-card";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { Executor, ExecutorType } from "@/lib/types/http";
import { EXECUTOR_ICON_MAP } from "@/lib/executor-icons";
import { useTranslation } from "react-i18next";

const EXECUTORS_ROUTE = "/settings/executors";
// The word the user must type to arm the delete button. It is compared with
// `!==`, so it must never be translated — the copy interpolates it instead, so
// the shown and compared values cannot drift.
const DELETE_CONFIRM_TOKEN = "delete";

export default function ExecutorEditPage({ executorId }: { executorId: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  const executor = useAppStore(
    (state) => state.executors.items.find((item: Executor) => item.id === executorId) ?? null,
  );

  if (!executor) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">{t("executors:executorNotFound")}</p>
            <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
              {t("executors:goToExecutors")}
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <ExecutorEditForm key={executor.id} executor={executor} />;
}

// Returns a catalog key rather than copy: this is module scope, so calling
// `t()` here would freeze the description at the boot locale. The caller
// resolves it at render.
function executorDescriptionKey(type: ExecutorType): string {
  if (type === "local_pc") return "executors:descriptionLocalPc";
  if (type === "worktree") return "executors:descriptionWorktree";
  if (type === "local_docker") return "executors:descriptionLocalDocker";
  if (type === "remote_docker") return "executors:descriptionRemoteDocker";
  if (type === "sprites") return "executors:descriptionSprites";
  if (type === "k8s") return "executors:descriptionKubernetes";
  return "executors:descriptionCustom";
}

function parseMcpPolicyJson(currentPolicy: string | undefined): Record<string, unknown> {
  let parsed: Record<string, unknown> = {};
  try {
    if (currentPolicy?.trim()) {
      parsed = JSON.parse(currentPolicy) as Record<string, unknown>;
    }
  } catch {
    parsed = {};
  }
  return parsed;
}

function McpPresetButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className="text-xs rounded-full border border-muted-foreground/30 px-2 py-1 hover:bg-muted cursor-pointer"
      onClick={onClick}
    >
      {label}
    </button>
  );
}

function McpPolicyCard({
  mcpPolicy,
  isDirty,
  mcpPolicyErrorKey,
  onPolicyChange,
}: {
  mcpPolicy: string;
  isDirty: boolean;
  mcpPolicyErrorKey: string | null;
  onPolicyChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const applyPreset = (updater: (parsed: Record<string, unknown>) => Record<string, unknown>) => {
    const parsed = parseMcpPolicyJson(mcpPolicy);
    const next = updater(parsed);
    onPolicyChange(JSON.stringify(next, null, 2));
  };

  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {t("executors:mcpPolicy")}
          <span className="rounded-full border border-muted-foreground/30 px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">
            {t("executors:advanced")}
          </span>
        </CardTitle>
        <CardDescription>{t("executors:mcpPolicyOverridesForExecutor")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Label htmlFor="mcp-policy">{t("executors:mcpPolicyJson")}</Label>
        <Textarea
          id="mcp-policy"
          value={mcpPolicy}
          data-settings-dirty={isDirty}
          onChange={(event) => onPolicyChange(event.target.value)}
          placeholder='{"allow_stdio":true,"allow_http":true}'
          rows={8}
        />
        {mcpPolicyErrorKey && <p className="text-xs text-destructive">{t(mcpPolicyErrorKey)}</p>}
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-xs font-medium text-muted-foreground">{t("executors:quickPresets")}</p>
          <McpPresetButton
            label={t("executors:onlyHttpSse")}
            onClick={() =>
              applyPreset((p) => ({ ...p, allow_stdio: false, allow_http: true, allow_sse: true }))
            }
          />
          <McpPresetButton
            label={t("executors:onlyStdio")}
            onClick={() =>
              applyPreset((p) => ({ ...p, allow_stdio: true, allow_http: false, allow_sse: false }))
            }
          />
          <McpPresetButton
            label={t("executors:allowlistGithubPlaywright")}
            onClick={() =>
              applyPreset((p) => {
                const existing = Array.isArray(p.allowlist_servers)
                  ? (p.allowlist_servers as string[])
                  : [];
                return {
                  ...p,
                  allowlist_servers: Array.from(new Set([...existing, "github", "playwright"])),
                };
              })
            }
          />
          <McpPresetButton
            label={t("executors:rewriteLocalhostForDocker")}
            onClick={() =>
              applyPreset((p) => {
                const existing =
                  p.url_rewrite && typeof p.url_rewrite === "object"
                    ? (p.url_rewrite as Record<string, string>)
                    : {};
                return {
                  ...p,
                  url_rewrite: {
                    ...existing,
                    "http://localhost:3000": "http://host.docker.internal:3000",
                  },
                };
              })
            }
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}

// Returns a catalog key (or null when valid) so the message resolves at render
// rather than at module load.
function validateMcpPolicy(value: string | undefined): string | null {
  const raw = value ?? "";
  if (!raw.trim()) return null;
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
      return "executors:mcpPolicyMustBeAJsonObject";
  } catch {
    return "executors:invalidJson";
  }
  return null;
}

function DeleteExecutorSection({ executor }: { executor: Executor }) {
  const { t } = useTranslation();
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDelete = async () => {
    if (deleteConfirmText !== DELETE_CONFIRM_TOKEN) return;
    setIsDeleting(true);
    try {
      const client = getWebSocketClient();
      if (client) {
        await client.request("executor.delete", { id: executor.id });
      } else {
        await deleteExecutorAction(executor.id);
      }
      setExecutors(executors.filter((item: Executor) => item.id !== executor.id));
      router.push(EXECUTORS_ROUTE);
    } finally {
      setIsDeleting(false);
      setDeleteDialogOpen(false);
    }
  };

  return (
    <>
      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">{t("executors:deleteExecutor")}</CardTitle>
        </CardHeader>
        <CardContent className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium">{t("executors:removeThisExecutor")}</p>
            <p className="text-xs text-muted-foreground">
              {t("executors:thisActionCannotBeUndone")}
            </p>
          </div>
          <Button
            variant="destructive"
            onClick={() => setDeleteDialogOpen(true)}
            className="cursor-pointer"
          >
            <IconTrash className="h-4 w-4 mr-2" />
            {t("executors:delete")}
          </Button>
        </CardContent>
      </Card>
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("executors:deleteExecutor")}</DialogTitle>
            <DialogDescription>
              {t("executors:typeTokenToConfirmDeletion", { token: DELETE_CONFIRM_TOKEN })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="confirm-delete">{t("executors:confirmDelete")}</Label>
            <Input
              id="confirm-delete"
              value={deleteConfirmText}
              onChange={(event) => setDeleteConfirmText(event.target.value)}
              placeholder={DELETE_CONFIRM_TOKEN}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteDialogOpen(false)}
              className="cursor-pointer"
            >
              {t("common:cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteConfirmText !== DELETE_CONFIRM_TOKEN || isDeleting}
              className="cursor-pointer"
            >
              {isDeleting ? t("executors:deleting") : t("executors:delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function ExecutorEditForm({ executor }: { executor: Executor }) {
  const { t } = useTranslation();
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [mcpPolicy, setMcpPolicy] = useState(executor.config?.mcp_policy ?? "");
  const [savedMcpPolicy, setSavedMcpPolicy] = useState(executor.config?.mcp_policy ?? "");

  const isSystem = executor.is_system ?? false;
  const ExecutorIcon = EXECUTOR_ICON_MAP[executor.type] ?? EXECUTOR_ICON_MAP.local;
  const mcpPolicyErrorKey = useMemo(() => validateMcpPolicy(mcpPolicy), [mcpPolicy]);
  const isDirty = mcpPolicy !== savedMcpPolicy;

  const handleSave = async () => {
    const config = { ...(executor.config ?? {}), mcp_policy: mcpPolicy };
    const payload = isSystem ? { config } : { name: executor.name, config };
    const client = getWebSocketClient();
    const updated = client
      ? await client.request<Executor>("executor.update", { id: executor.id, ...payload })
      : await updateExecutorAction(executor.id, payload);
    setSavedMcpPolicy(updated.config?.mcp_policy ?? "");
    setExecutors(
      executors.map((item: Executor) => (item.id === updated.id ? { ...item, ...updated } : item)),
    );
  };
  useSettingsSaveContributor({
    id: `executor:${executor.id}`,
    revision: mcpPolicy,
    isDirty,
    canSave: !mcpPolicyErrorKey,
    invalidReason: mcpPolicyErrorKey ? t(mcpPolicyErrorKey) : undefined,
    save: handleSave,
    discard: () => setMcpPolicy(savedMcpPolicy),
  });

  return (
    <div className="space-y-8">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <ExecutorIcon className="h-5 w-5 text-muted-foreground" />
            <h2 className="min-w-0 break-words text-2xl font-bold">{executor.name}</h2>
          </div>
          <p className="text-sm text-muted-foreground mt-1">
            {t(executorDescriptionKey(executor.type))}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className="min-h-11 w-full cursor-pointer text-sm md:min-h-7 md:w-auto md:text-xs"
        >
          {t("executors:backToExecutors")}
        </Button>
      </div>
      <Separator />

      <ExecutorProfilesCard executorId={executor.id} profiles={executor.profiles ?? []} />

      <McpPolicyCard
        mcpPolicy={mcpPolicy}
        isDirty={isDirty}
        mcpPolicyErrorKey={mcpPolicyErrorKey}
        onPolicyChange={setMcpPolicy}
      />

      {!isSystem && <DeleteExecutorSection executor={executor} />}
    </div>
  );
}
