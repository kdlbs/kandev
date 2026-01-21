'use client';

import { Suspense, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { IconCloud, IconServer } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@kandev/ui/card';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Separator } from '@kandev/ui/separator';
import { createExecutorAction } from '@/app/actions/executors';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import type { Executor } from '@/lib/types/http';

const EXECUTOR_TYPES = ['local_docker'] as const;
type ExecutorType = (typeof EXECUTOR_TYPES)[number];

export default function ExecutorCreatePage() {
  return (
    <Suspense fallback={<div className="p-4">Loading...</div>}>
      <ExecutorCreatePageContent />
    </Suspense>
  );
}

function ExecutorCreatePageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const initialType = searchParams.get('type');
  const [type, setType] = useState<ExecutorType>(() => {
    if (EXECUTOR_TYPES.includes(initialType as ExecutorType)) {
      return initialType as ExecutorType;
    }
    return 'local_docker';
  });
  const [name, setName] = useState('Local Docker');
  const [dockerHost, setDockerHost] = useState('unix:///var/run/docker.sock');
  const [isCreating, setIsCreating] = useState(false);
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);

  const handleCreate = async () => {
    if (type !== 'local_docker') {
      return;
    }
    setIsCreating(true);
    try {
      const payload = {
        name,
        type,
        status: 'active',
        config: { docker_host: dockerHost },
      };
      const client = getWebSocketClient();
      const created = client
        ? await client.request<Executor>('executor.create', payload)
        : await createExecutorAction(payload);
      setExecutors([...executors.filter((item) => item.id !== created.id), created]);
      router.push('/settings/executors');
    } finally {
      setIsCreating(false);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">Create Executor</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Choose an executor type to run environments on your infrastructure.
        </p>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {type === 'local_docker' ? <IconServer className="h-4 w-4" /> : <IconCloud className="h-4 w-4" />}
            {type === 'local_docker' ? 'Local Docker Executor' : 'Remote Docker Executor'}
          </CardTitle>
          <CardDescription>
            {type === 'local_docker'
              ? 'Uses the local Docker daemon on this machine.'
              : 'Connects to a remote Docker host (coming soon).'}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="executor-type">Executor type</Label>
            <Select value={type} onValueChange={(value) => setType(value as ExecutorType)}>
              <SelectTrigger id="executor-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="local_docker">Local Docker</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="executor-name">Executor name</Label>
            <Input
              id="executor-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="docker-host">Docker host env value</Label>
            <Input
              id="docker-host"
              value={dockerHost}
              onChange={(event) => setDockerHost(event.target.value)}
              placeholder="unix:///var/run/docker.sock"
              disabled={type !== 'local_docker'}
            />
            <p className="text-xs text-muted-foreground">
              Repositories will be mounted as volumes at runtime.
            </p>
          </div>
        </CardContent>
      </Card>

      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" onClick={() => router.push('/settings/executors')}>
          Cancel
        </Button>
        <Button onClick={handleCreate} disabled={isCreating || type !== 'local_docker'}>
          {isCreating ? 'Creating...' : 'Create Executor'}
        </Button>
      </div>
    </div>
  );
}
