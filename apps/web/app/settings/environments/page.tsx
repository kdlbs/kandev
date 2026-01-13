'use client';

import Link from 'next/link';
import { IconPlus } from '@tabler/icons-react';
import { useEffect, useState } from 'react';
import { Button } from '@kandev/ui/button';
import { Separator } from '@kandev/ui/separator';
import { EnvironmentCard } from '@/components/settings/environment-card';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import { listEnvironmentsAction } from '@/app/actions/environments';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Environment } from '@/lib/types/http';

export default function EnvironmentsSettingsPage() {
  const [environments, setEnvironments] = useState<Environment[]>([]);

  useEffect(() => {
    const client = getWebSocketClient();
    const fallback = () =>
      SETTINGS_DATA.environments.map((env) => ({
        id: env.id,
        name: env.name,
        kind: env.kind,
        worktree_root: env.worktreeRoot,
        image_tag: env.imageTag,
        dockerfile: env.dockerfile,
        build_config: env.buildConfig
          ? {
              base_image: env.buildConfig.baseImage,
              install_agents: env.buildConfig.installAgents.join(','),
            }
          : undefined,
        created_at: '',
        updated_at: '',
      }));
    if (client) {
      client
        .request<{ environments: Environment[] }>('environment.list', {})
        .then((resp) => setEnvironments(resp.environments))
        .catch(() => setEnvironments(fallback()));
      return;
    }
    listEnvironmentsAction()
      .then((resp) => setEnvironments(resp.environments))
      .catch(() => setEnvironments(fallback()));
  }, []);

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">Environments</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Configure runtime environments for agent sessions.
          </p>
        </div>
        <Button asChild size="sm">
          <Link href="/settings/environment/new">
            <IconPlus className="h-4 w-4 mr-2" />
            Create Custom Environment
          </Link>
        </Button>
      </div>

      <Separator />

      <div className="grid gap-3">
        {environments.map((env) => (
          <EnvironmentCard key={env.id} environment={env} />
        ))}
      </div>
    </div>
  );
}
