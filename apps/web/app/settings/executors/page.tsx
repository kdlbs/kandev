'use client';

import Link from 'next/link';
import { IconChevronRight, IconBrandDocker, IconCloud } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { Card, CardContent } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { useAppStore } from '@/components/state-provider';
import type { Executor } from '@/lib/types/http';
import { EXECUTOR_ICON_MAP, getExecutorLabel } from '@/lib/executor-icons';

const CREATION_OPTIONS = [
  {
    id: 'local_docker',
    label: 'Local Docker',
    description: 'Run on the local Docker daemon.',
    href: '/settings/executor/new?type=local_docker',
    icon: IconBrandDocker,
    enabled: true,
  },
  {
    id: 'remote_docker',
    label: 'Remote Docker',
    description: 'Connect to a remote Docker host.',
    href: '/settings/executor/new?type=remote_docker',
    icon: IconCloud,
    enabled: true,
  },
];

type CreationOptionCardProps = {
  option: (typeof CREATION_OPTIONS)[number];
};

function CreationOptionCard({ option }: CreationOptionCardProps) {
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
      <div className="cursor-not-allowed opacity-60">
        <Card className="h-full">{cardBody}</Card>
      </div>
    );
  }
  return (
    <Link href={option.href} className="block">
      <Card className="h-full hover:bg-accent transition-colors">
        {cardBody}
      </Card>
    </Link>
  );
}

function ExecutorTypeDescription({ type }: { type: string }) {
  if (type === 'local') {
    return (
      <div className="text-xs text-muted-foreground mt-1">
        Runs agents directly in the repository folder. No worktree isolation.
      </div>
    );
  }
  if (type === 'worktree') {
    return (
      <div className="text-xs text-muted-foreground mt-1">
        Creates a git worktree for each session. Agents work in isolated branches.
      </div>
    );
  }
  return null;
}

const DefaultIcon = EXECUTOR_ICON_MAP.local;

function ExecutorIconBadge({ type }: { type: string }) {
  const Icon = EXECUTOR_ICON_MAP[type] ?? DefaultIcon;
  return (
    <div className="p-2 bg-muted rounded-md">
      <Icon className="h-4 w-4" />
    </div>
  );
}

function ExecutorListItem({ executor }: { executor: Executor }) {
  const typeLabel = getExecutorLabel(executor.type);
  const showDockerHost = (executor.type === 'local_docker' || executor.type === 'remote_docker') && executor.config?.docker_host;

  return (
    <Link href={`/settings/executor/${executor.id}`}>
      <Card className="hover:bg-accent transition-colors cursor-pointer">
        <CardContent className="py-4">
          <div className="flex items-start justify-between">
            <div className="flex items-start gap-3 flex-1">
              <ExecutorIconBadge type={executor.type} />
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
                {showDockerHost && (
                  <div className="text-xs text-muted-foreground mt-1">
                    {executor.config?.docker_host}
                  </div>
                )}
                <ExecutorTypeDescription type={executor.type} />
              </div>
            </div>
            <IconChevronRight className="h-5 w-5 text-muted-foreground" />
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

export default function ExecutorsSettingsPage() {
  const executors = useAppStore((state) => state.executors.items);

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
          {CREATION_OPTIONS.map((option) => (
            <CreationOptionCard key={option.id} option={option} />
          ))}
        </div>
      </div>

      <div className="grid gap-3">
        {executors.map((executor: Executor) => (
          <ExecutorListItem key={executor.id} executor={executor} />
        ))}
      </div>
    </div>
  );
}
