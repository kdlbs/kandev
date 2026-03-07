"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import { IconPlayerPlay, IconLoader2, IconCheck, IconX, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
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

const DEFAULT_IMAGE_TAG = "kandev/agent:latest";
const DEFAULT_DOCKERFILE = "FROM ubuntu:24.04\n";

type BuildStatus = "idle" | "building" | "success" | "failed";

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
  if (status === "idle") return null;
  if (status === "building") return <Badge variant="secondary">Building...</Badge>;
  if (status === "success") {
    return (
      <Badge variant="default" className="bg-green-600">
        <IconCheck className="mr-1 h-3 w-3" />
        Success
      </Badge>
    );
  }
  return (
    <Badge variant="destructive">
      <IconX className="mr-1 h-3 w-3" />
      Failed
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

function useBuildStream() {
  const [buildStatus, setBuildStatus] = useState<BuildStatus>("idle");
  const [buildLog, setBuildLog] = useState("");

  const appendLog = useCallback((text: string) => {
    setBuildLog((prev) => prev + text);
  }, []);

  const runBuild = useCallback(
    async (dockerfile: string, tag: string) => {
      setBuildStatus("building");
      setBuildLog("");
      try {
        const response = await buildDockerImage({ dockerfile, tag });
        if (!response.ok) {
          const text = await response.text();
          setBuildStatus("failed");
          setBuildLog(`Build failed (${response.status}): ${text}`);
          return;
        }
        const reader = response.body?.getReader();
        if (!reader) {
          setBuildStatus("failed");
          setBuildLog("No response body");
          return;
        }
        const hasError = await readDockerStream(reader, appendLog);
        setBuildStatus(hasError ? "failed" : "success");
      } catch (err) {
        setBuildStatus("failed");
        const msg = err instanceof Error ? err.message : "Unknown error";
        setBuildLog((prev) => prev + `\nBuild failed: ${msg}`);
      }
    },
    [appendLog],
  );

  return { buildStatus, buildLog, runBuild };
}

type DockerfileBuildCardProps = {
  dockerfile: string;
  onDockerfileChange: (v: string) => void;
  imageTag: string;
  onImageTagChange: (v: string) => void;
};

export function DockerfileBuildCard({
  dockerfile,
  onDockerfileChange,
  imageTag,
  onImageTagChange,
}: DockerfileBuildCardProps) {
  const { buildStatus, buildLog, runBuild } = useBuildStream();
  const logRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [buildLog]);

  const handleBuild = () => {
    if (dockerfile.trim() && imageTag.trim()) void runBuild(dockerfile, imageTag);
  };

  const canFillDefaults = !dockerfile.trim() || !imageTag.trim();

  const fillDefaults = () => {
    if (!imageTag.trim()) onImageTagChange(DEFAULT_IMAGE_TAG);
    if (!dockerfile.trim()) onDockerfileChange(DEFAULT_DOCKERFILE);
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <CardTitle>Dockerfile</CardTitle>
            <CardDescription>Define the Docker image. Build and test it here.</CardDescription>
          </div>
          {canFillDefaults && (
            <Button
              variant="ghost"
              size="sm"
              onClick={fillDefaults}
              className="cursor-pointer text-xs text-muted-foreground"
            >
              Use defaults
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="image-tag">Image Tag</Label>
          <Input
            id="image-tag"
            value={imageTag}
            onChange={(e) => onImageTagChange(e.target.value)}
            placeholder={DEFAULT_IMAGE_TAG}
            className="font-mono text-sm"
          />
        </div>
        <div className="space-y-2">
          <Label>Dockerfile Content</Label>
          <div className="overflow-hidden rounded-md border">
            <ScriptEditor
              value={dockerfile}
              onChange={onDockerfileChange}
              language="dockerfile"
              height="250px"
            />
          </div>
        </div>
        <div className="flex items-center gap-3">
          <Button
            onClick={handleBuild}
            disabled={buildStatus === "building" || !dockerfile.trim() || !imageTag.trim()}
            className="cursor-pointer"
          >
            {buildStatus === "building" ? (
              <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" />
            ) : (
              <IconPlayerPlay className="mr-1.5 h-4 w-4" />
            )}
            Build Image
          </Button>
          <BuildStatusBadge status={buildStatus} />
        </div>
        {buildLog && (
          <pre
            ref={logRef}
            className="max-h-[300px] overflow-auto rounded-md bg-black p-3 font-mono text-xs text-green-400"
          >
            {buildLog}
          </pre>
        )}
      </CardContent>
    </Card>
  );
}

function ContainersEmptyState({ loading }: { loading: boolean }) {
  if (loading) {
    return (
      <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
        <IconLoader2 className="h-4 w-4 animate-spin" />
        Loading...
      </div>
    );
  }
  return <p className="py-4 text-sm text-muted-foreground">No running containers.</p>;
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
  const isLoading = actionLoading === container.id;
  return (
    <TableRow>
      <TableCell className="font-mono text-sm">{container.name}</TableCell>
      <TableCell className="text-sm">{container.image}</TableCell>
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
              title="Stop"
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
            title="Remove"
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

export function DockerContainersCard({ profileId }: { profileId: string }) {
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const result = await listDockerContainers({ labels: { "kandev.profile_id": profileId } });
      setContainers(result.containers ?? []);
    } catch {
      setContainers([]);
    } finally {
      setLoading(false);
    }
  }, [profileId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

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
      <CardHeader>
        <CardTitle>Running Containers</CardTitle>
        <CardDescription>Docker containers created by this profile.</CardDescription>
      </CardHeader>
      <CardContent>
        {containers.length === 0 ? (
          <ContainersEmptyState loading={loading} />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Image</TableHead>
                <TableHead>Status</TableHead>
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
