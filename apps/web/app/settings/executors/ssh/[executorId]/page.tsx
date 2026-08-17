"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { IconTerminal2 } from "@tabler/icons-react";
import { useAppStoreApi } from "@/components/state-provider";
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

export default function SSHExecutorPage({ executorId }: { executorId: string }) {
  return <SSHExecutorPageContent key={executorId} executorId={executorId} />;
}

function SSHExecutorPageContent({ executorId }: { executorId: string }) {
  const { t } = useTranslation();
  const { executor, loading, error, reload } = useExecutor(executorId);

  if (loading) {
    return (
      <Card>
        <CardContent className="py-12 text-center text-sm text-muted-foreground">
          {t("executors:loadingExecutor")}
        </CardContent>
      </Card>
    );
  }
  if (error || !executor) {
    return <NotFoundCard message={error ?? t("executors:executorNotFound")} />;
  }
  if (executor.type !== "ssh") {
    // The executor id is an identifier the user may need to look up, so it is
    // interpolated as a value rather than written into the message.
    return <NotFoundCard message={t("executors:notAnSshExecutor", { id: executor.id })} />;
  }
  return <SSHExecutorView executor={executor} onSaved={reload} />;
}

function useExecutor(executorId: string) {
  const { t } = useTranslation();
  const [executor, setExecutor] = useState<LoadedExecutor | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestGeneration = useRef(0);

  const load = useCallback(async () => {
    const generation = ++requestGeneration.current;
    setExecutor(null);
    setLoading(true);
    setError(null);
    try {
      const res = await fetchExecutor(executorId);
      if (generation === requestGeneration.current) {
        setExecutor(res);
      }
    } catch (e) {
      if (generation === requestGeneration.current) {
        setError(e instanceof Error ? e.message : t("executors:failedToLoadExecutor"));
      }
    } finally {
      if (generation === requestGeneration.current) {
        setLoading(false);
      }
    }
  }, [executorId, t]);

  useEffect(() => {
    void load();
    return () => {
      requestGeneration.current += 1;
    };
  }, [load]);

  return { executor, loading, error, reload: load };
}

function NotFoundCard({ message }: { message: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="text-muted-foreground">{message}</p>
        <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
          {t("executors:backToExecutors")}
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
        coordinatedSaveId={`ssh-executor:${executor.id}`}
        runningSessionCount={sessionCount}
      />
      <SSHSessionsCard executorId={executor.id} />
    </div>
  );
}

function SSHExecutorHeader({ executorName }: { executorName: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <>
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <IconTerminal2 className="h-5 w-5 text-muted-foreground" />
            <h2 className="min-w-0 break-words text-2xl font-bold">{executorName}</h2>
            <Badge variant="outline" className="text-[10px]">
              {getExecutorLabel("ssh")}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("executors:sshExecutorPageDescription")}
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
  const store = useAppStoreApi();

  return useCallback(
    async (cfg: SSHExecutorConfig) => {
      const config = buildSSHExecutorConfig(cfg);
      await updateExecutor(executorId, { name: cfg.name, config });
      // Refresh the store so the executor list reflects the new name + config.
      try {
        const fresh = await listExecutors();
        store.getState().setExecutors(fresh.executors);
      } catch {
        // Non-fatal: the local view still reloads via onSaved(). Read the
        // current snapshot at write time so a WS event that updated the
        // executor list mid-flight doesn't get overwritten with a stale
        // captured copy.
        const current = store.getState().executors.items;
        store
          .getState()
          .setExecutors(
            current.map((e: Executor) =>
              e.id === executorId ? { ...e, name: cfg.name, config } : e,
            ),
          );
      }
      await onSaved();
    },
    [executorId, store, onSaved],
  );
}
