'use client';

import { use, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconCpu, IconServer, IconTrash } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@kandev/ui/card';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Separator } from '@kandev/ui/separator';
import { Textarea } from '@kandev/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import { updateExecutorAction, deleteExecutorAction } from '@/app/actions/executors';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Executor } from '@/lib/types/http';
import { useAppStore } from '@/components/state-provider';

export default function ExecutorEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const executor = useAppStore((state) => state.executors.items.find((item) => item.id === id) ?? null);

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

  return <ExecutorEditForm key={executor.id} executor={executor} />;
}

type ExecutorEditFormProps = {
  executor: Executor;
};

function ExecutorEditForm({ executor }: ExecutorEditFormProps) {
  const router = useRouter();
  const [draft, setDraft] = useState<Executor>({ ...executor });
  const [savedExecutor, setSavedExecutor] = useState<Executor>({ ...executor });
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const isSystem = draft.is_system ?? false;
  const isLocalDocker = draft.type === 'local_docker';

  const ExecutorIcon = useMemo(() => {
    return draft.type === 'local_pc' ? IconCpu : IconServer;
  }, [draft.type]);

  const isDirty = useMemo(() => {
    return (
      draft.name !== savedExecutor.name ||
      draft.type !== savedExecutor.type ||
      draft.status !== savedExecutor.status ||
      JSON.stringify(draft.config ?? {}) !== JSON.stringify(savedExecutor.config ?? {})
    );
  }, [draft, savedExecutor]);

  const handleSaveExecutor = async () => {
    if (!draft) return;
    setIsSaving(true);
    try {
      const payload = {
        name: draft.name,
        type: draft.type,
        status: draft.status,
        config: draft.config ?? {},
      };
      const client = getWebSocketClient();
      const updated = client
        ? await client.request<Executor>('executor.update', { id: draft.id, ...payload })
        : await updateExecutorAction(draft.id, payload);
      setDraft(updated);
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
        await client.request('executor.delete', { id: draft.id });
      } else {
        await deleteExecutorAction(draft.id);
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
          <h2 className="text-2xl font-bold">{draft.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            {draft.type === 'local_pc'
              ? 'Uses locally installed agents on this machine.'
              : draft.type === 'local_docker'
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
            <span>{draft.type}</span>
          </div>
          <div className="space-y-2">
            <Label htmlFor="executor-name">Executor name</Label>
            <Input
              id="executor-name"
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.target.value })}
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
              value={draft.config?.docker_host ?? ''}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  config: { ...(draft.config ?? {}), docker_host: event.target.value },
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

      {draft.type === 'remote_docker' && (
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

      <Card>
        <CardHeader>
          <CardTitle>MCP Policy</CardTitle>
          <CardDescription>JSON policy overrides for MCP servers on this executor.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          <Label htmlFor="mcp-policy">MCP policy JSON</Label>
          <Textarea
            id="mcp-policy"
            value={draft.config?.mcp_policy ?? ''}
            onChange={(event) =>
              setDraft({
                ...draft,
                config: { ...(draft.config ?? {}), mcp_policy: event.target.value },
              })
            }
            placeholder='{"allow_stdio":true,"allow_http":true,"allowlist_servers":["github"],"url_rewrite":{"http://localhost:3000":"http://mcp-svc:3000"}}'
            rows={8}
          />
          <p className="text-xs text-muted-foreground">
            Leave empty to use the default policy for the runtime.
          </p>
        </CardContent>
      </Card>

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
              <CardTitle className="text-destructive">Delete Executor</CardTitle>
            </CardHeader>
            <CardContent className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">Remove this executor</p>
                <p className="text-xs text-muted-foreground">This action cannot be undone.</p>
              </div>
              <Button variant="destructive" onClick={() => setDeleteDialogOpen(true)}>
                <IconTrash className="h-4 w-4 mr-2" />
                Delete
              </Button>
            </CardContent>
          </Card>

          <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Delete Executor</DialogTitle>
                <DialogDescription>
                Type &quot;delete&quot; to confirm deletion. This action cannot be undone.
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-2">
                <Label htmlFor="confirm-delete">Confirm Delete</Label>
                <Input
                  id="confirm-delete"
                  value={deleteConfirmText}
                  onChange={(event) => setDeleteConfirmText(event.target.value)}
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
                  disabled={deleteConfirmText !== 'delete' || isDeleting}
                >
                  {isDeleting ? 'Deleting...' : 'Delete'}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </>
      )}
    </div>
  );
}
