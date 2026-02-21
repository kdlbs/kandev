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
import { ExecutorProfilesCard } from "@/components/settings/executor-profiles-card";
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
            <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
              Go to Executors
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <ExecutorEditForm key={executor.id} executor={executor} />;
}

function getExecutorDescription(type: string): string {
  if (type === "local") return "Runs agents directly in the repository folder.";
  if (type === "worktree") return "Creates git worktrees for isolated agent sessions.";
  if (type === "local_docker") return "Runs Docker containers on this machine.";
  if (type === "remote_docker") return "Connects to a remote Docker host.";
  if (type === "sprites") return "Runs agents in Sprites.dev cloud sandboxes.";
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
  mcpPolicyError,
  onPolicyChange,
}: {
  mcpPolicy: string;
  mcpPolicyError: string | null;
  onPolicyChange: (value: string) => void;
}) {
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
          JSON policy overrides for MCP servers on this executor.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Label htmlFor="mcp-policy">MCP policy JSON</Label>
        <Textarea
          id="mcp-policy"
          value={mcpPolicy}
          onChange={(event) => onPolicyChange(event.target.value)}
          placeholder='{"allow_stdio":true,"allow_http":true}'
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
      </CardContent>
    </Card>
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

function DeleteExecutorSection({ executor }: { executor: Executor }) {
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDelete = async () => {
    if (deleteConfirmText !== "delete") return;
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
          <CardTitle className="text-destructive">Delete Executor</CardTitle>
        </CardHeader>
        <CardContent className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium">Remove this executor</p>
            <p className="text-xs text-muted-foreground">This action cannot be undone.</p>
          </div>
          <Button
            variant="destructive"
            onClick={() => setDeleteDialogOpen(true)}
            className="cursor-pointer"
          >
            <IconTrash className="h-4 w-4 mr-2" />
            Delete
          </Button>
        </CardContent>
      </Card>
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
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
              value={deleteConfirmText}
              onChange={(event) => setDeleteConfirmText(event.target.value)}
              placeholder="delete"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)} className="cursor-pointer">
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteConfirmText !== "delete" || isDeleting}
              className="cursor-pointer"
            >
              {isDeleting ? "Deleting..." : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function ExecutorEditForm({ executor }: { executor: Executor }) {
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [mcpPolicy, setMcpPolicy] = useState(executor.config?.mcp_policy ?? "");
  const [savedMcpPolicy, setSavedMcpPolicy] = useState(executor.config?.mcp_policy ?? "");
  const [isSaving, setIsSaving] = useState(false);

  const isSystem = executor.is_system ?? false;
  const ExecutorIcon = EXECUTOR_ICON_MAP[executor.type] ?? EXECUTOR_ICON_MAP.local;
  const mcpPolicyError = useMemo(() => validateMcpPolicy(mcpPolicy), [mcpPolicy]);
  const isDirty = mcpPolicy !== savedMcpPolicy;

  const handleSave = async () => {
    setIsSaving(true);
    try {
      const config = { ...(executor.config ?? {}), mcp_policy: mcpPolicy };
      const payload = isSystem ? { config } : { name: executor.name, config };
      const client = getWebSocketClient();
      const updated = client
        ? await client.request<Executor>("executor.update", { id: executor.id, ...payload })
        : await updateExecutorAction(executor.id, payload);
      setSavedMcpPolicy(updated.config?.mcp_policy ?? "");
      setExecutors(
        executors.map((item: Executor) =>
          item.id === updated.id ? { ...item, ...updated } : item,
        ),
      );
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <div className="flex items-center gap-2">
            <ExecutorIcon className="h-5 w-5 text-muted-foreground" />
            <h2 className="text-2xl font-bold">{executor.name}</h2>
          </div>
          <p className="text-sm text-muted-foreground mt-1">{getExecutorDescription(executor.type)}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => router.push(EXECUTORS_ROUTE)} className="cursor-pointer">
          Back to Executors
        </Button>
      </div>
      <Separator />

      <ExecutorProfilesCard executorId={executor.id} profiles={executor.profiles ?? []} />

      <McpPolicyCard
        mcpPolicy={mcpPolicy}
        mcpPolicyError={mcpPolicyError}
        onPolicyChange={setMcpPolicy}
      />

      {isDirty && (
        <div className="flex items-center justify-end gap-2">
          <Button
            variant="outline"
            onClick={() => setMcpPolicy(savedMcpPolicy)}
            className="cursor-pointer"
          >
            Discard
          </Button>
          <Button
            onClick={handleSave}
            disabled={Boolean(mcpPolicyError) || isSaving}
            className="cursor-pointer"
          >
            {isSaving ? "Saving..." : "Save Changes"}
          </Button>
        </div>
      )}

      {!isSystem && <DeleteExecutorSection executor={executor} />}
    </div>
  );
}
