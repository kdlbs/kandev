"use client";

import { useCallback } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { IconTerminal2 } from "@tabler/icons-react";
import { useExecutorsQuerySync } from "@/hooks/domains/settings/use-executors-query-sync";
import { createExecutor, createExecutorProfile } from "@/lib/api/domains/settings-api";
import { SSHConnectionCard } from "@/components/settings/ssh-connection-card";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { getExecutorLabel } from "@/lib/executor-icons";
import { buildSSHExecutorConfig } from "./ssh-config";
import type { Executor } from "@/lib/types/http";

const EXECUTORS_ROUTE = "/settings/executors";

/**
 * SSH-specific "new executor" flow. Renders just the SSHConnectionCard;
 * Save POSTs to /api/v1/executors with type=ssh and the freshly-pinned
 * fingerprint, then immediately creates a default profile under it (the
 * /settings/executors index lists profiles, so an executor with no
 * profile would otherwise be invisible). Routes to the profile-edit page
 * after both writes succeed — that's where the user picks the shell,
 * probes for installed agents, and tunes per-task config (workdir,
 * prepare script, credentials). The host-level executor page
 * (/settings/executors/ssh/:id) remains reachable for re-trust and
 * sessions inspection.
 */
export function SSHCreatePage() {
  const router = useRouter();
  const { upsertExecutor } = useExecutorsQuerySync();

  const handleSave = useCallback(
    async (cfg: SSHExecutorConfig) => {
      const created = await createExecutor({
        name: cfg.name,
        type: "ssh",
        config: buildSSHExecutorConfig(cfg),
      });
      // Auto-create a default profile so the executor shows up in the
      // /settings/executors list (which flattens by profile). The profile
      // carries the same name as the executor and inherits the connection
      // config; users can add more profiles later if they want different
      // workdir roots / prepare scripts / env vars on the same host.
      const profile = await createExecutorProfile(created.id, {
        name: cfg.name,
      });
      const next: Executor = {
        id: created.id,
        name: created.name,
        type: "ssh",
        status: "active",
        is_system: false,
        config: created.config,
        profiles: [profile],
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      upsertExecutor(next);
      router.push(`/settings/executors/${profile.id}`);
    },
    [router, upsertExecutor],
  );

  return (
    <div className="space-y-8">
      <SSHCreateHeader />
      <SSHConnectionCard onSave={handleSave} />
    </div>
  );
}

function SSHCreateHeader() {
  const router = useRouter();
  return (
    <>
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <div className="flex items-center gap-2">
            <IconTerminal2 className="h-5 w-5 text-muted-foreground" />
            <h2 className="text-2xl font-bold">New SSH Executor</h2>
            <Badge variant="outline" className="text-xs">
              {getExecutorLabel("ssh")}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            Connect to a remote Linux amd64 or macOS host and run agentctl there.
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
