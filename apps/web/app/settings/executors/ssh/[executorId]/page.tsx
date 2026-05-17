"use client";

import { use, useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { IconTerminal2 } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { fetchExecutor, listExecutors, updateExecutor } from "@/lib/api/domains/settings-api";
import { SSHConnectionCard } from "@/components/settings/ssh-connection-card";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { SSHSessionsCard } from "@/components/settings/ssh-sessions-card";
import { listSSHSessions } from "@/lib/api/domains/ssh-api";
import { getExecutorLabel } from "@/lib/executor-icons";
import {
  buildSSHExecutorConfig,
  parseSSHExecutorConfig,
} from "@/app/settings/executors/new/[type]/ssh-config";
import type { Executor } from "@/lib/types/http";

const EXECUTORS_ROUTE = "/settings/executors";

type LoadedExecutor = {
  id: string;
  name: string;
  type: string;
  config?: Record<string, string>;
};

export default function SSHExecutorPage({ params }: { params: Promise<{ executorId: string }> }) {
  const { executorId } = use(params);
  const { executor, loading, error, reload } = useExecutor(executorId);

  if (loading) {
    return (
      <Card>
        <CardContent className="py-12 text-center text-sm text-muted-foreground">
          Loading executor...
        </CardContent>
      </Card>
    );
  }
  if (error || !executor) {
    return <NotFoundCard message={error ?? "Executor not found"} />;
  }
  if (executor.type !== "ssh") {
    return <NotFoundCard message={`Executor ${executor.id} is not an SSH executor`} />;
  }
  return <SSHExecutorView executor={executor} onSaved={reload} />;
}

function useExecutor(executorId: string) {
  const [executor, setExecutor] = useState<LoadedExecutor | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetchExecutor(executorId);
      setExecutor(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load executor");
    } finally {
      setLoading(false);
    }
  }, [executorId]);

  useEffect(() => {
    void load();
  }, [load]);

  return { executor, loading, error, reload: load };
}

function NotFoundCard({ message }: { message: string }) {
  const router = useRouter();
  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="text-muted-foreground">{message}</p>
        <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
          Back to Executors
        </Button>
      </CardContent>
    </Card>
  );
}

function SSHExecutorView({
  executor,
  onSaved,
}: {
  executor: LoadedExecutor;
  onSaved: () => void | Promise<void>;
}) {
  const initial = parseSSHExecutorConfig(executor.name, executor.config);
  const sessionCount = useRunningSessionCount(executor.id);
  const handleSave = useSaveExecutor(executor.id, onSaved);

  return (
    <div className="space-y-8">
      <SSHExecutorHeader executorName={executor.name} />
      <SSHConnectionCard
        // The `key` forces a fresh form state when the executor reloads with
        // updated config so the user sees the new pinned fingerprint.
        key={`${executor.id}:${executor.config?.ssh_host_fingerprint ?? "none"}`}
        initial={initial}
        onSave={handleSave}
        runningSessionCount={sessionCount}
      />
      <SSHSessionsCard executorId={executor.id} />
    </div>
  );
}

function SSHExecutorHeader({ executorName }: { executorName: string }) {
  const router = useRouter();
  return (
    <>
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <div className="flex items-center gap-2">
            <IconTerminal2 className="h-5 w-5 text-muted-foreground" />
            <h2 className="text-2xl font-bold">{executorName}</h2>
            <Badge variant="outline" className="text-xs">
              {getExecutorLabel("ssh")}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            Edit the connection settings or re-trust the host. Existing sessions keep their snapshot
            of the previous config.
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

function useRunningSessionCount(executorId: string): number {
  const [count, setCount] = useState(0);
  useEffect(() => {
    let cancelled = false;
    listSSHSessions(executorId)
      .then((rows) => {
        if (!cancelled) setCount(rows.length);
      })
      .catch(() => {
        if (!cancelled) setCount(0);
      });
    return () => {
      cancelled = true;
    };
  }, [executorId]);
  return count;
}

function useSaveExecutor(executorId: string, onSaved: () => void | Promise<void>) {
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);

  return useCallback(
    async (cfg: SSHExecutorConfig) => {
      const config = buildSSHExecutorConfig(cfg);
      await updateExecutor(executorId, { name: cfg.name, config });
      // Refresh the store so the executor list reflects the new name + config.
      try {
        const fresh = await listExecutors();
        setExecutors(fresh.executors);
      } catch {
        // Non-fatal: the local view still reloads via onSaved().
        setExecutors(
          executors.map((e: Executor) =>
            e.id === executorId ? { ...e, name: cfg.name, config } : e,
          ),
        );
      }
      await onSaved();
    },
    [executorId, executors, setExecutors, onSaved],
  );
}
