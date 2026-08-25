"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import { IconPlayerPlay, IconLoader2, IconCheck, IconX, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { ScriptEditor } from "@/components/settings/profile-edit/script-editor";
import {
  buildDockerImage,
  listDockerContainers,
  stopDockerContainer,
  removeDockerContainer,
} from "@/lib/api/domains/settings-api";
import type { DockerContainer } from "@/lib/api/domains/settings-api";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useTranslation } from "react-i18next";

const DEFAULT_IMAGE_TAG = "kandev/multi-agent:latest";
// Self-contained default that produces a working image:
//   - node + npm/npx so ACP agents (Claude, Codex, OpenCode, Auggie) can be
//     fetched on demand at runtime
//   - git so the prepare script can clone the workspace into the container
//   - ca-certificates + curl as a baseline for any extra tooling users add
//
// The kandev backend mounts the agentctl binary into /usr/local/bin/agentctl
// at container creation time, so users do NOT need to bake it in here.
// i18n-exempt: Dockerfile contents, not prose.
const DEFAULT_DOCKERFILE = `FROM node:22-slim

RUN apt-get update \\
    && apt-get install -y --no-install-recommends \\
       git ca-certificates curl \\
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
`;

export type BuildStatus = "idle" | "building" | "success" | "failed";
export type DockerBuildSuccess = { dockerfile: string; imageTag: string };

/** Parse a Docker JSON stream line into a human-readable string. */
function parseDockerLine(raw: string): string | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  try {
    const obj = JSON.parse(trimmed) as Record<string, unknown>;
    if (obj.error) return `ERROR: ${obj.error}\n`;
    if (typeof obj.stream === "string") return obj.stream;
    if (typeof obj.status === "string") {
      const id = obj.id ? ` ${obj.id}` : "";
      return `${obj.status}${id}\n`;
    }
    // Skip metadata-only messages (aux, progressDetail, etc.)
    return null;
  } catch {
    return trimmed;
  }
}

function BuildStatusBadge({ status }: { status: BuildStatus }) {
  const { t } = useTranslation();
  if (status === "idle") return null;
  if (status === "building") return <Badge variant="secondary">{t("executors:building")}</Badge>;
  if (status === "success") {
    return (
      <Badge variant="default" className="bg-green-600">
        <IconCheck className="mr-1 h-3 w-3" />
        {t("executors:success")}
      </Badge>
    );
  }
  return (
    <Badge variant="destructive">
      <IconX className="mr-1 h-3 w-3" />
      {t("executors:failed")}
    </Badge>
  );
}

async function readDockerStream(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  appendLog: (text: string) => void,
): Promise<boolean> {
  const decoder = new TextDecoder();
  let buffer = "";
  let hasError = false;
  let done = false;
  while (!done) {
    const result = await reader.read();
    done = result.done;
    if (!result.value) continue;
    buffer += decoder.decode(result.value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      const parsed = parseDockerLine(line);
      if (!parsed) continue;
      if (parsed.startsWith("ERROR:")) hasError = true;
      appendLog(parsed);
    }
  }
  const tail = parseDockerLine(buffer);
  if (tail) appendLog(tail);
  return hasError;
}

function useBuildStream(onBuildSuccess?: (result: DockerBuildSuccess) => void) {
  const { t } = useTranslation();
  const [buildStatus, setBuildStatus] = useState<BuildStatus>("idle");
  const [buildLog, setBuildLog] = useState("");

  const appendLog = useCallback((text: string) => {
    setBuildLog((prev) => prev + text);
  }, []);

  // Only the lines this client writes into the log pane are translated. The
  // rest of the pane is `docker build`'s own stdout, streamed through
  // `parseDockerLine` verbatim.
  const runBuild = useCallback(
    async (dockerfile: string, tag: string) => {
      setBuildStatus("building");
      setBuildLog("");
      try {
        const response = await buildDockerImage({ dockerfile, tag });
        if (!response.ok) {
          const text = await response.text();
          setBuildStatus("failed");
          setBuildLog(t("executors:buildFailedWithStatus", { status: response.status, text }));
          return;
        }
        const reader = response.body?.getReader();
        if (!reader) {
          setBuildStatus("failed");
          setBuildLog(t("executors:noResponseBody"));
          return;
        }
        const hasError = await readDockerStream(reader, appendLog);
        const nextStatus = hasError ? "failed" : "success";
        setBuildStatus(nextStatus);
        if (nextStatus === "success") {
          onBuildSuccess?.({ dockerfile, imageTag: tag.trim() });
        }
      } catch (err) {
        setBuildStatus("failed");
        const msg = err instanceof Error ? err.message : t("executors:unknownError");
        setBuildLog(
          (prev) => prev + `\n${t("executors:buildFailedWithMessage", { message: msg })}`,
        );
      }
    },
    [appendLog, onBuildSuccess, t],
  );

  return { buildStatus, buildLog, runBuild };
}

type DockerfileBuildCardProps = {
  dockerfile: string;
  onDockerfileChange: (v: string) => void;
  imageTag: string;
  baselineDockerfile?: string;
  baselineImageTag?: string;
  onImageTagChange: (v: string) => void;
  onBuildSuccess?: (result: DockerBuildSuccess) => void;
};

/** Build trigger plus its status badge, or the admin-only explanation. */
function BuildActionRow({
  canBuild,
  buildStatus,
  disabled,
  onBuild,
}: {
  canBuild: boolean;
  buildStatus: BuildStatus;
  disabled: boolean;
  onBuild: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-3">
      <Button onClick={onBuild} disabled={disabled} className="cursor-pointer">
        {buildStatus === "building" ? (
          <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" />
        ) : (
          <IconPlayerPlay className="mr-1.5 h-4 w-4" />
        )}
        {t("executors:buildImage")}
      </Button>
      {canBuild ? (
        <BuildStatusBadge status={buildStatus} />
      ) : (
        <p className="text-sm text-muted-foreground">{t("executors:buildImageAdminOnly")}</p>
      )}
    </div>
  );
}

export function DockerfileBuildCard({
  dockerfile,
  onDockerfileChange,
  imageTag,
  baselineDockerfile,
  baselineImageTag,
  onImageTagChange,
  onBuildSuccess,
}: DockerfileBuildCardProps) {
  const { t } = useTranslation();
  const { buildStatus, buildLog, runBuild } = useBuildStream(onBuildSuccess);
  const logRef = useRef<HTMLPreElement>(null);
  // Building an image is a host-level operation with no per-user resource, so
  // the backend gates POST /api/v1/docker/build on the admin role.
  const canBuild = useIsAdmin();

  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [buildLog]);

  const handleBuild = () => {
    if (canBuild && dockerfile.trim() && imageTag.trim()) void runBuild(dockerfile, imageTag);
  };

  const canFillDefaults = !dockerfile.trim() || !imageTag.trim();
  const dockerfileDirty = baselineDockerfile !== undefined && dockerfile !== baselineDockerfile;
  const imageTagDirty = baselineImageTag !== undefined && imageTag !== baselineImageTag;

  const fillDefaults = () => {
    if (!imageTag.trim()) onImageTagChange(DEFAULT_IMAGE_TAG);
    if (!dockerfile.trim()) onDockerfileChange(DEFAULT_DOCKERFILE);
  };

  return (
    <SettingsCard isDirty={dockerfileDirty || imageTagDirty}>
      <SettingsCardHeader
        title={t("executors:dockerfile")}
        description={t("executors:defineTheDockerImageBuildAnd")}
        actions={
          canFillDefaults ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={fillDefaults}
              className={settingsActionClassName("cursor-pointer text-muted-foreground")}
            >
              {t("executors:useDefaults")}
            </Button>
          ) : undefined
        }
      />
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="image-tag">{t("executors:imageTag")}</Label>
          <Input
            id="image-tag"
            value={imageTag}
            onChange={(e) => onImageTagChange(e.target.value)}
            placeholder={DEFAULT_IMAGE_TAG}
            className="font-mono text-sm"
            data-settings-dirty={imageTagDirty}
          />
        </div>
        <div className="space-y-2">
          <Label>{t("executors:dockerfileContent")}</Label>
          <div
            className="overflow-hidden rounded-md border"
            data-settings-dirty={dockerfileDirty}
            data-settings-dirty-level="container"
          >
            <ScriptEditor
              value={dockerfile}
              onChange={onDockerfileChange}
              language="dockerfile"
              height="250px"
            />
          </div>
        </div>
        <BuildActionRow
          canBuild={canBuild}
          buildStatus={buildStatus}
          disabled={
            !canBuild || buildStatus === "building" || !dockerfile.trim() || !imageTag.trim()
          }
          onBuild={handleBuild}
        />
        {buildLog && (
          <pre
            ref={logRef}
            className="max-h-[300px] overflow-auto rounded-md bg-black p-3 font-mono text-xs text-green-400"
          >
            {buildLog}
          </pre>
        )}
      </CardContent>
    </SettingsCard>
  );
}

function ContainersEmptyState({ loading }: { loading: boolean }) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
        <IconLoader2 className="h-4 w-4 animate-spin" />
        {t("executors:loading")}
      </div>
    );
  }
  return <p className="py-4 text-sm text-muted-foreground">{t("executors:noDockerContainers")}</p>;
}

export function useDockerProfileContainers(profileId: string, enabled = true) {
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [loading, setLoading] = useState(enabled);

  const refresh = useCallback(async () => {
    if (!enabled) {
      setContainers([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const result = await listDockerContainers({
        labels: { "kandev.executor_profile_id": profileId },
      });
      setContainers(result.containers ?? []);
    } catch {
      setContainers([]);
    } finally {
      setLoading(false);
    }
  }, [enabled, profileId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { containers, loading, refresh };
}

/**
 * The container's task title, falling back to its task id. Returns `null` when
 * neither label is present so the caller can render translated copy — the
 * fallback is the only part of this value that is copy rather than user data.
 */
export function containerTaskLabel(container: DockerContainer): string | null {
  const title = container.labels?.["kandev.task_title"]?.trim();
  return title || container.labels?.["kandev.task_id"] || null;
}

function ContainerRow({
  container,
  actionLoading,
  onStop,
  onRemove,
}: {
  container: DockerContainer;
  actionLoading: string | null;
  onStop: (id: string) => void;
  onRemove: (id: string) => void;
}) {
  const { t } = useTranslation();
  const isLoading = actionLoading === container.id;
  return (
    <TableRow data-testid={`docker-container-row-${container.id}`}>
      <TableCell className="font-mono text-sm">{container.name}</TableCell>
      <TableCell className="text-sm">{container.image}</TableCell>
      <TableCell className="text-sm" data-testid="docker-container-task">
        {containerTaskLabel(container) ?? t("executors:untrackedTask")}
      </TableCell>
      <TableCell>
        <Badge variant={container.state === "running" ? "default" : "secondary"}>
          {container.status}
        </Badge>
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-1">
          {container.state === "running" && (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onStop(container.id)}
              disabled={isLoading}
              className="cursor-pointer"
              title={t("executors:stop")}
            >
              {isLoading ? (
                <IconLoader2 className="h-4 w-4 animate-spin" />
              ) : (
                <IconX className="h-4 w-4" />
              )}
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onRemove(container.id)}
            disabled={isLoading}
            className="cursor-pointer"
            title={t("executors:remove")}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

export function DockerContainersCard({ profileId }: { profileId: string }) {
  const { t } = useTranslation();
  const { containers, loading, refresh } = useDockerProfileContainers(profileId);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const handleAction = useCallback(
    async (id: string, action: (id: string) => Promise<void>) => {
      setActionLoading(id);
      try {
        await action(id);
        await refresh();
      } finally {
        setActionLoading(null);
      }
    },
    [refresh],
  );

  return (
    <Card>
      <SettingsCardHeader
        title={t("executors:dockerContainers")}
        description={t("executors:dockerContainersCreatedByThisProfile")}
      />
      <CardContent>
        {containers.length === 0 ? (
          <ContainersEmptyState loading={loading} />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("executors:name")}</TableHead>
                <TableHead>{t("executors:image")}</TableHead>
                <TableHead>{t("executors:task")}</TableHead>
                <TableHead>{t("common:status")}</TableHead>
                <TableHead className="w-[100px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {containers.map((c) => (
                <ContainerRow
                  key={c.id}
                  container={c}
                  actionLoading={actionLoading}
                  onStop={(id) => handleAction(id, stopDockerContainer)}
                  onRemove={(id) => handleAction(id, removeDockerContainer)}
                />
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
