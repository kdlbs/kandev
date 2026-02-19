"use client";

import { use, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
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
import type { Executor } from "@/lib/types/http";
import { EXECUTOR_ICON_MAP } from "@/lib/executor-icons";

const EXECUTORS_ROUTE = "/settings/executors";

export default function ExecutorEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const executor = useAppStore(
    (state) => state.executors.items.find((item: Executor) => item.id === id) ?? null,
  );

  if (!executor) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Executor not found</p>
            <Button className="mt-4" onClick={() => router.push(EXECUTORS_ROUTE)}>
              Go to Executors
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <ExecutorEditForm key={executor.id} executor={executor} />;
}

type ExecutorEditFormProps = {
  executor: Executor;
};

function getExecutorDescription(type: string): string {
  if (type === "local") return "Runs agents directly in the repository folder.";
  if (type === "worktree") return "Creates git worktrees for isolated agent sessions.";
  if (type === "local_docker") return "Runs Docker containers on this machine.";
  if (type === "remote_docker") return "Connects to a remote Docker host.";
  return "Custom executor.";
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

type McpPresetButtonProps = {
  label: string;
  onClick: () => void;
};

function McpPresetButton({ label, onClick }: McpPresetButtonProps) {
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

type McpPolicyCardProps = {
  mcpPolicy: string;
  mcpPolicyError: string | null;
  onPolicyChange: (value: string) => void;
};

function McpPolicyCard({ mcpPolicy, mcpPolicyError, onPolicyChange }: McpPolicyCardProps) {
  const applyPreset = (updater: (parsed: Record<string, unknown>) => Record<string, unknown>) => {
    const parsed = parseMcpPolicyJson(mcpPolicy);
    const next = updater(parsed);
    onPolicyChange(JSON.stringify(next, null, 2));
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          MCP Policy
          <span className="rounded-full border border-muted-foreground/30 px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">
            Advanced
          </span>
        </CardTitle>
        <CardDescription>
          JSON policy overrides for MCP servers on this executor. Use it to enforce transport rules,
          restrict which servers can run, or rewrite URLs for this runtime.
          <p className="text-xs text-muted-foreground mt-2">
            Examples: block stdio on remote runners, allowlist approved servers, or rewrite
            localhost URLs for Docker/K8s networking.
          </p>
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Label htmlFor="mcp-policy">MCP policy JSON</Label>
        <Textarea
          id="mcp-policy"
          value={mcpPolicy}
          onChange={(event) => onPolicyChange(event.target.value)}
          placeholder='{"allow_stdio":true,"allow_http":true,"allowlist_servers":["github"],"url_rewrite":{"http://localhost:3000":"http://mcp-svc:3000"}}'
          rows={8}
        />
        {mcpPolicyError && <p className="text-xs text-destructive">{mcpPolicyError}</p>}
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-xs font-medium text-muted-foreground">Quick presets</p>
          <McpPresetButton
            label="Only HTTP/SSE"
            onClick={() =>
              applyPreset((p) => ({ ...p, allow_stdio: false, allow_http: true, allow_sse: true }))
            }
          />
          <McpPresetButton
            label="Only stdio"
            onClick={() =>
              applyPreset((p) => ({ ...p, allow_stdio: true, allow_http: false, allow_sse: false }))
            }
          />
          <McpPresetButton
            label="Allowlist GitHub + Playwright"
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
            label="Rewrite localhost for Docker"
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
        <p className="text-xs text-muted-foreground">
          Leave empty to use the default policy for the runtime (local runs allow stdio + HTTP +
          SSE, and apply no allowlist or URL rewrites).
        </p>
      </CardContent>
    </Card>
  );
}

type DeleteExecutorDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: () => void;
  isDeleting: boolean;
  confirmText: string;
  onConfirmTextChange: (value: string) => void;
};

function DeleteExecutorDialog({
  open,
  onOpenChange,
  onDelete,
  isDeleting,
  confirmText,
  onConfirmTextChange,
}: DeleteExecutorDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Executor</DialogTitle>
          <DialogDescription>
            Type &quot;delete&quot; to confirm deletion. This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="confirm-delete">Confirm Delete</Label>
          <Input
            id="confirm-delete"
            value={confirmText}
            onChange={(event) => onConfirmTextChange(event.target.value)}
            placeholder="delete"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={onDelete}
            disabled={confirmText !== "delete" || isDeleting}
          >
            {isDeleting ? "Deleting..." : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type ExecutorDetailsCardProps = {
  draft: Executor;
  isSystem: boolean;
  onNameChange: (value: string) => void;
};

function ExecutorDetailsCard({ draft, isSystem, onNameChange }: ExecutorDetailsCardProps) {
  const ExecutorIcon = EXECUTOR_ICON_MAP[draft.type] ?? EXECUTOR_ICON_MAP.local;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Executor Details</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <ExecutorIcon className="h-4 w-4" />
          <span>{draft.type}</span>
        </div>
        <div className="space-y-2">
          <Label htmlFor="executor-name">Executor name</Label>
          <Input
            id="executor-name"
            value={draft.name}
            onChange={(event) => onNameChange(event.target.value)}
            disabled={isSystem}
          />
          {isSystem && (
            <p className="text-xs text-muted-foreground">System executor names cannot be edited.</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function DockerConfigCard({
  draft,
  onDraftChange,
}: {
  draft: Executor;
  onDraftChange: (next: Executor) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Docker Configuration</CardTitle>
        <CardDescription>
          {draft.type === "remote_docker"
            ? "Remote Docker host settings."
            : "Local Docker host settings."}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Label htmlFor="docker-host">Docker host env value</Label>
        <Input
          id="docker-host"
          value={draft.config?.docker_host ?? ""}
          onChange={(event) =>
            onDraftChange({
              ...draft,
              config: { ...(draft.config ?? {}), docker_host: event.target.value },
            })
          }
          placeholder="unix:///var/run/docker.sock"
        />
        <p className="text-xs text-muted-foreground">
          The repository will be mounted as a volume during runtime.
        </p>
      </CardContent>
    </Card>
  );
}

function DeleteExecutorSection({
  onDeleteClick,
  deleteDialogOpen,
  setDeleteDialogOpen,
  onDelete,
  isDeleting,
  confirmText,
  onConfirmTextChange,
}: {
  onDeleteClick: () => void;
  deleteDialogOpen: boolean;
  setDeleteDialogOpen: (open: boolean) => void;
  onDelete: () => void;
  isDeleting: boolean;
  confirmText: string;
  onConfirmTextChange: (value: string) => void;
}) {
  return (
    <>
      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">Delete Executor</CardTitle>
        </CardHeader>
        <CardContent className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium">Remove this executor</p>
            <p className="text-xs text-muted-foreground">This action cannot be undone.</p>
          </div>
          <Button variant="destructive" onClick={onDeleteClick}>
            <IconTrash className="h-4 w-4 mr-2" />
            Delete
          </Button>
        </CardContent>
      </Card>
      <DeleteExecutorDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        onDelete={onDelete}
        isDeleting={isDeleting}
        confirmText={confirmText}
        onConfirmTextChange={onConfirmTextChange}
      />
    </>
  );
}

function validateMcpPolicy(value: string | undefined): string | null {
  const raw = value ?? "";
  if (!raw.trim()) return null;
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
      return "MCP policy must be a JSON object";
  } catch {
    return "Invalid JSON";
  }
  return null;
}

function ExecutorEditForm({ executor }: ExecutorEditFormProps) {
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [draft, setDraft] = useState<Executor>({ ...executor });
  const [savedExecutor, setSavedExecutor] = useState<Executor>({ ...executor });
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const isSystem = draft.is_system ?? false;
  const isDockerType = draft.type === "local_docker" || draft.type === "remote_docker";
  const mcpPolicyError = useMemo(
    () => validateMcpPolicy(draft.config?.mcp_policy),
    [draft.config?.mcp_policy],
  );

  const isDirty = useMemo(
    () =>
      draft.name !== savedExecutor.name ||
      draft.type !== savedExecutor.type ||
      draft.status !== savedExecutor.status ||
      JSON.stringify(draft.config ?? {}) !== JSON.stringify(savedExecutor.config ?? {}),
    [draft, savedExecutor],
  );

  const handleSaveExecutor = async () => {
    if (!draft) return;
    setIsSaving(true);
    try {
      const payload = isSystem
        ? { config: draft.config ?? {} }
        : { name: draft.name, type: draft.type, status: draft.status, config: draft.config ?? {} };
      const client = getWebSocketClient();
      const updated = client
        ? await client.request<Executor>("executor.update", { id: draft.id, ...payload })
        : await updateExecutorAction(draft.id, payload);
      setDraft(updated);
      setSavedExecutor(updated);
      setExecutors(
        executors.map((item: Executor) =>
          item.id === updated.id ? { ...item, ...updated } : item,
        ),
      );
    } finally {
      setIsSaving(false);
    }
  };

  const handleDeleteExecutor = async () => {
    if (deleteConfirmText !== "delete") return;
    setIsDeleting(true);
    try {
      const client = getWebSocketClient();
      if (client) {
        await client.request("executor.delete", { id: draft.id });
      } else {
        await deleteExecutorAction(draft.id);
      }
      setExecutors(executors.filter((item: Executor) => item.id !== draft.id));
      router.push(EXECUTORS_ROUTE);
    } finally {
      setIsDeleting(false);
      setDeleteDialogOpen(false);
    }
  };

  const handleMcpPolicyChange = (value: string) => {
    setDraft({ ...draft, config: { ...(draft.config ?? {}), mcp_policy: value } });
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <h2 className="text-2xl font-bold">{draft.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">{getExecutorDescription(draft.type)}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => router.push(EXECUTORS_ROUTE)}>
          Back to Executors
        </Button>
      </div>
      <Separator />
      <ExecutorDetailsCard
        draft={draft}
        isSystem={isSystem}
        onNameChange={(value) => setDraft({ ...draft, name: value })}
      />
      {isDockerType && <DockerConfigCard draft={draft} onDraftChange={setDraft} />}
      <McpPolicyCard
        mcpPolicy={draft.config?.mcp_policy ?? ""}
        mcpPolicyError={mcpPolicyError}
        onPolicyChange={handleMcpPolicyChange}
      />
      <Separator />
      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" onClick={() => router.push(EXECUTORS_ROUTE)}>
          Cancel
        </Button>
        <Button
          onClick={handleSaveExecutor}
          disabled={!isDirty || Boolean(mcpPolicyError) || isSaving}
        >
          {isSaving ? "Saving..." : "Save Changes"}
        </Button>
      </div>
      {!isSystem && (
        <DeleteExecutorSection
          onDeleteClick={() => setDeleteDialogOpen(true)}
          deleteDialogOpen={deleteDialogOpen}
          setDeleteDialogOpen={setDeleteDialogOpen}
          onDelete={handleDeleteExecutor}
          isDeleting={isDeleting}
          confirmText={deleteConfirmText}
          onConfirmTextChange={setDeleteConfirmText}
        />
      )}
    </div>
  );
}
