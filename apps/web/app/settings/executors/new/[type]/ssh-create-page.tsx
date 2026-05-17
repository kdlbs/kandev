"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { IconTerminal2 } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { createExecutor } from "@/lib/api/domains/settings-api";
import { SSHConnectionCard } from "@/components/settings/ssh-connection-card";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { getExecutorLabel } from "@/lib/executor-icons";
import { buildSSHExecutorConfig } from "./ssh-config";
import type { Executor } from "@/lib/types/http";

const EXECUTORS_ROUTE = "/settings/executors";

/**
 * SSH-specific "new executor" flow. Renders just the SSHConnectionCard;
 * Save POSTs to /api/v1/executors with type=ssh and the freshly-pinned
 * fingerprint, then routes to the existing-executor SSH page.
 *
 * Profiles for the SSH executor are still managed via the standard profile
 * flow once the executor itself exists.
 */
export function SSHCreatePage() {
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);

  const handleSave = useCallback(
    async (cfg: SSHExecutorConfig) => {
      const created = await createExecutor({
        name: cfg.name,
        type: "ssh",
        config: buildSSHExecutorConfig(cfg),
      });
      const next: Executor = {
        id: created.id,
        name: created.name,
        type: created.type,
        status: "active",
        is_system: false,
        config: created.config,
        profiles: [],
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      setExecutors([...executors, next]);
      router.push(`/settings/executors/ssh/${created.id}`);
    },
    [router, executors, setExecutors],
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
            Connect to a remote host over SSH and run agentctl there. linux/amd64 only in v1.
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
