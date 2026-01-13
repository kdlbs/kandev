'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { IconChevronRight, IconCloud, IconCpu, IconServer } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { Card, CardContent } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { listExecutorsAction } from '@/app/actions/executors';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import type { Executor } from '@/lib/types/http';
import { getWebSocketClient } from '@/lib/ws/connection';

export default function ExecutorsSettingsPage() {
  const [executors, setExecutors] = useState<Executor[]>([]);
  const creationOptions = [
    {
      id: 'local_docker',
      label: 'Local Docker',
      description: 'Run on the local Docker daemon.',
      href: '/settings/executor/new?type=local_docker',
      icon: IconServer,
      enabled: true,
    },
    {
      id: 'remote_docker',
      label: 'Remote Docker',
      description: 'Connect to a remote Docker host.',
      href: '/settings/executor/new?type=remote_docker',
      icon: IconCloud,
      enabled: false,
    },
    {
      id: 'remote_server',
      label: 'Remote Server',
      description: 'SSH into a dedicated host.',
      href: '/settings/executor/new?type=remote_server',
      icon: IconCloud,
      enabled: false,
    },
    {
      id: 'k8s',
      label: 'K8s',
      description: 'Run workloads in a Kubernetes cluster.',
      href: '/settings/executor/new?type=k8s',
      icon: IconCloud,
      enabled: false,
    },
  ];

  useEffect(() => {
    const client = getWebSocketClient();
    const fallback = () =>
      SETTINGS_DATA.executors.map((executor) => ({
        id: executor.id,
        name: executor.name,
        type: executor.type,
        status: executor.status,
        is_system: executor.isSystem,
        config: executor.config,
        created_at: '',
        updated_at: '',
      }));
    if (client) {
      client
        .request<{ executors: Executor[] }>('executor.list', {})
        .then((resp) => setExecutors(resp.executors))
        .catch(() => setExecutors(fallback()));
      return;
    }
    listExecutorsAction()
      .then((resp) => setExecutors(resp.executors))
      .catch(() => setExecutors(fallback()));
  }, []);

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">Executors</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Choose where environments run today and prepare for remote targets.
          </p>
        </div>
      </div>

      <Separator />

      <div>
        <div className="text-sm font-medium mb-3">Create an executor</div>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {creationOptions.map((option) => {
            const Icon = option.icon;
            const cardBody = (
              <CardContent className="py-4">
                <div className="flex items-start gap-3">
                  <div className="p-2 bg-muted rounded-md">
                    <Icon className="h-4 w-4" />
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{option.label}</span>
                      {!option.enabled && (
                        <Badge variant="outline" className="text-xs">
                          Coming soon
                        </Badge>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground mt-1">{option.description}</p>
                  </div>
                </div>
              </CardContent>
            );
            if (!option.enabled) {
              return (
                <div key={option.id} className="cursor-not-allowed opacity-60">
                  <Card className="h-full">{cardBody}</Card>
                </div>
              );
            }
            return (
              <Link key={option.id} href={option.href} className="block">
                <Card className="h-full hover:bg-accent transition-colors">
                  {cardBody}
                </Card>
              </Link>
            );
          })}
        </div>
      </div>

      <div className="grid gap-3">
        {executors.map((executor) => {
          const Icon = executor.type === 'local_pc' ? IconCpu : IconServer;
          const typeLabel =
            executor.type === 'local_pc'
              ? 'Local PC'
              : executor.type === 'local_docker'
              ? 'Local Docker'
              : 'Remote Docker';
          return (
            <Link key={executor.id} href={`/settings/executor/${executor.id}`}>
              <Card className="hover:bg-accent transition-colors cursor-pointer">
                <CardContent className="py-4">
                  <div className="flex items-start justify-between">
                    <div className="flex items-start gap-3 flex-1">
                      <div className="p-2 bg-muted rounded-md">
                        <Icon className="h-4 w-4" />
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <h4 className="font-medium">{executor.name}</h4>
                          <Badge variant="secondary" className="text-xs">
                            {typeLabel}
                          </Badge>
                          {executor.status === 'disabled' && (
                            <Badge variant="outline" className="text-xs">
                              Disabled
                            </Badge>
                          )}
                        </div>
                        {executor.type === 'local_docker' && executor.config?.docker_host && (
                          <div className="text-xs text-muted-foreground mt-1">
                            {executor.config.docker_host}
                          </div>
                        )}
                      </div>
                    </div>
                    <IconChevronRight className="h-5 w-5 text-muted-foreground" />
                  </div>
                </CardContent>
              </Card>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
