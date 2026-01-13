'use client';

import { use, useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconCpu, IconServer, IconTrash } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@kandev/ui/card';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Separator } from '@kandev/ui/separator';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import { getExecutorAction, updateExecutorAction, deleteExecutorAction } from '@/app/actions/executors';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Executor } from '@/lib/types/http';

export default function ExecutorEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [executor, setExecutor] = useState<Executor | null>(null);
  const [savedExecutor, setSavedExecutor] = useState<Executor | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  useEffect(() => {
    getExecutorAction(id)
      .then((data) => {
        setExecutor(data);
        setSavedExecutor(data);
      })
      .catch(() => setExecutor(null))
      .finally(() => setIsLoading(false));
  }, [id]);

  const isSystem = executor?.is_system ?? false;
  const isLocalDocker = executor?.type === 'local_docker';

  const ExecutorIcon = useMemo(() => {
    return executor?.type === 'local_pc' ? IconCpu : IconServer;
  }, [executor?.type]);

  const isDirty = useMemo(() => {
    if (!executor || !savedExecutor) return false;
    return (
      executor.name !== savedExecutor.name ||
      executor.type !== savedExecutor.type ||
      executor.status !== savedExecutor.status ||
      JSON.stringify(executor.config ?? {}) !== JSON.stringify(savedExecutor.config ?? {})
    );
  }, [executor, savedExecutor]);

  if (isLoading) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Loading executor...</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (!executor) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Executor not found</p>
            <Button className="mt-4" onClick={() => router.push('/settings/executors')}>
              Go to Executors
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const handleSaveExecutor = async () => {
    if (!executor) return;
    setIsSaving(true);
    try {
      const payload = {
        name: executor.name,
        type: executor.type,
        status: executor.status,
        config: executor.config ?? {},
      };
      const client = getWebSocketClient();
      const updated = client
        ? await client.request<Executor>('executor.update', { id: executor.id, ...payload })
        : await updateExecutorAction(executor.id, payload);
      setExecutor(updated);
      setSavedExecutor(updated);
      router.push('/settings/executors');
    } finally {
      setIsSaving(false);
    }
  };

  const handleDeleteExecutor = async () => {
    if (deleteConfirmText !== 'delete') return;
    setIsDeleting(true);
    try {
      const client = getWebSocketClient();
      if (client) {
        await client.request('executor.delete', { id: executor.id });
      } else {
        await deleteExecutorAction(executor.id);
      }
      router.push('/settings/executors');
    } finally {
      setIsDeleting(false);
      setDeleteDialogOpen(false);
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <h2 className="text-2xl font-bold">{executor.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            {executor.type === 'local_pc'
              ? 'Uses locally installed agents on this machine.'
              : executor.type === 'local_docker'
              ? 'Runs Docker containers on this machine.'
              : 'Remote Docker support is coming soon.'}
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => router.push('/settings/executors')}>
          Back to Executors
        </Button>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle>Executor Details</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <ExecutorIcon className="h-4 w-4" />
            <span>{executor.type}</span>
          </div>
          <div className="space-y-2">
            <Label htmlFor="executor-name">Executor name</Label>
            <Input
              id="executor-name"
              value={executor.name}
              onChange={(event) => setExecutor({ ...executor, name: event.target.value })}
              disabled={isSystem}
            />
            {isSystem && (
              <p className="text-xs text-muted-foreground">System executor names cannot be edited.</p>
            )}
          </div>
        </CardContent>
      </Card>

      {isLocalDocker && (
        <Card>
          <CardHeader>
            <CardTitle>Docker Configuration</CardTitle>
            <CardDescription>Local Docker host settings.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            <Label htmlFor="docker-host">Docker host env value</Label>
            <Input
              id="docker-host"
              value={executor.config?.docker_host ?? ''}
              onChange={(event) =>
                setExecutor({
                  ...executor,
                  config: { ...(executor.config ?? {}), docker_host: event.target.value },
                })
              }
              placeholder="unix:///var/run/docker.sock"
            />
            <p className="text-xs text-muted-foreground">
              The repository will be mounted as a volume during runtime.
            </p>
          </CardContent>
        </Card>
      )}

      {executor.type === 'remote_docker' && (
        <Card>
          <CardHeader>
            <CardTitle>Remote Docker</CardTitle>
            <CardDescription>Coming soon.</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Remote Docker executors will support SSH and cloud runtimes in a future update.
            </p>
          </CardContent>
        </Card>
      )}

      <Separator />

      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" onClick={() => router.push('/settings/executors')}>
          Cancel
        </Button>
        <Button onClick={handleSaveExecutor} disabled={!isDirty || isSystem || isSaving}>
          {isSaving ? 'Saving...' : 'Save Changes'}
        </Button>
      </div>

      {!isSystem && (
        <>
          <Card className="border-destructive">
            <CardHeader>
              <CardTitle className="text-destructive">Danger Zone</CardTitle>
              <CardDescription>Irreversible actions that permanently delete this executor.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Delete this executor</p>
                  <p className="text-sm text-muted-foreground">
                    Once deleted, environments using this executor will need to be updated.
                  </p>
                </div>
                <Button variant="destructive" onClick={() => setDeleteDialogOpen(true)}>
                  <IconTrash className="h-4 w-4 mr-2" />
                  Delete Executor
                </Button>
              </div>
            </CardContent>
          </Card>

          <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Delete Executor</DialogTitle>
                <DialogDescription>
                  This action cannot be undone. This will permanently delete the executor &quot;
                  {executor.name}&quot;.
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4 py-4">
                <p className="text-sm">
                  Please type <span className="font-mono font-bold">delete</span> to confirm:
                </p>
                <Input
                  value={deleteConfirmText}
                  onChange={(e) => setDeleteConfirmText(e.target.value)}
                  placeholder="delete"
                />
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
                  Cancel
                </Button>
                <Button
                  variant="destructive"
                  onClick={handleDeleteExecutor}
                  disabled={deleteConfirmText !== 'delete'}
                >
                  {isDeleting ? 'Deleting...' : 'Delete Executor'}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </>
      )}
    </div>
  );
}
