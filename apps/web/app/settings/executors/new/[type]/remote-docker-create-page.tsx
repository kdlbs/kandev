"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { IconBrandDocker } from "@tabler/icons-react";
import { useAppStoreApi } from "@/components/state-provider";
import {
  createExecutor,
  createExecutorProfile,
  deleteExecutor,
} from "@/lib/api/domains/settings-api";
import { SSHConnectionCard } from "@/components/settings/ssh-connection-card";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { testRemoteDockerConnection } from "@/lib/api/domains/remote-docker-api";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { buildSSHExecutorConfig } from "./ssh-config";
import type { Executor } from "@/lib/types/http";

const EXECUTORS_ROUTE = "/settings/executors";

/**
 * Remote Docker "new executor" flow.
 *
 * The connection is an SSH target, so this reuses the SSH connection card and
 * only swaps the endpoint: the remote Docker test adds the daemon and
 * API-version steps on the same connection. Save stores the pinned
 * fingerprint, then creates a default profile so the executor appears in the
 * hub, which lists profiles rather than executors.
 */
export function RemoteDockerCreatePage() {
  const router = useRouter();
  const store = useAppStoreApi();

  const handleSave = useCallback(
    async (cfg: SSHExecutorConfig) => {
      const created = await createExecutor({
        name: cfg.name,
        type: "remote_docker",
        config: buildSSHExecutorConfig(cfg),
      });
      // The hub lists profiles, not bare executors, so an executor whose
      // default profile failed is stored but invisible: the administrator sees
      // only a failed save, and retrying adds a duplicate. Roll it back.
      let profile;
      try {
        profile = await createExecutorProfile(created.id, { name: cfg.name });
      } catch (cause) {
        await deleteExecutor(created.id).catch(() => undefined);
        throw cause;
      }
      const next: Executor = {
        id: created.id,
        name: created.name,
        type: "remote_docker",
        status: "active",
        is_system: false,
        config: created.config,
        profiles: [profile],
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      const current = store.getState().executors.items;
      const merged = current.some((e) => e.id === next.id) ? current : [...current, next];
      store.getState().setExecutors(merged);
      router.push(`/settings/executors/${profile.id}`);
    },
    [router, store],
  );

  return (
    <div className="space-y-8">
      <RemoteDockerCreateHeader />
      <RemoteDockerAuthorityNotice />
      <SSHConnectionCard onSave={handleSave} testConnection={testRemoteDockerConnection} />
    </div>
  );
}

/**
 * States the trust boundary where the profile is configured, not only in the
 * docs: the SSH user must reach the remote Docker socket, and Dockerfile
 * instructions execute with the daemon's authority.
 */
function RemoteDockerAuthorityNotice() {
  const { t } = useTranslation();
  return (
    <div
      className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm"
      data-testid="remote-docker-authority-notice"
    >
      {t("executors:remoteDockerAuthorityNotice")}
    </div>
  );
}

function RemoteDockerCreateHeader() {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <>
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <IconBrandDocker className="h-5 w-5 text-muted-foreground" />
            <h2 className="min-w-0 break-words text-2xl font-bold">
              {t("executors:newRemoteDockerExecutor")}
            </h2>
            <Badge variant="outline" className="text-[10px]">
              {t("executors:remoteDocker")}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("executors:remoteDockerCreateDescription")}
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className={settingsActionClassName("w-full cursor-pointer text-sm md:w-auto md:text-xs")}
        >
          {t("executors:backToExecutors")}
        </Button>
      </div>
      <Separator />
    </>
  );
}
