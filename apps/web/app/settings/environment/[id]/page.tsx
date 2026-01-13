'use client';

import { use, useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconCube, IconTrash } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@kandev/ui/card';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
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
import { getEnvironmentAction, updateEnvironmentAction, deleteEnvironmentAction } from '@/app/actions/environments';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Environment } from '@/lib/types/http';
import type { BaseDocker } from '@/lib/settings/types';

const BASE_IMAGE_LABELS: Record<BaseDocker, string> = {
  universal: 'Universal (Ubuntu)',
  golang: 'Golang',
  node: 'Node.js',
  python: 'Python',
};

export default function EnvironmentEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [environment, setEnvironment] = useState<Environment | null>(null);
  const [savedEnvironment, setSavedEnvironment] = useState<Environment | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  useEffect(() => {
    getEnvironmentAction(id)
      .then((data) => {
        setEnvironment(data);
        setSavedEnvironment(data);
      })
      .catch(() => setEnvironment(null))
      .finally(() => setIsLoading(false));
  }, [id]);

  const isDirty = useMemo(() => {
    if (!environment || !savedEnvironment) return false;
    return (
      environment.name !== savedEnvironment.name ||
      environment.kind !== savedEnvironment.kind ||
      environment.worktree_root !== savedEnvironment.worktree_root ||
      environment.image_tag !== savedEnvironment.image_tag ||
      environment.dockerfile !== savedEnvironment.dockerfile ||
      JSON.stringify(environment.build_config ?? {}) !== JSON.stringify(savedEnvironment.build_config ?? {})
    );
  }, [environment, savedEnvironment]);

  if (isLoading) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Loading environment...</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (!environment) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Environment not found</p>
            <Button className="mt-4" onClick={() => router.push('/settings/environments')}>
              Go to Environments
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const handleDeleteEnvironment = async () => {
    if (deleteConfirmText !== 'delete') return;
    setIsDeleting(true);
    try {
      const client = getWebSocketClient();
      if (client) {
        await client.request('environment.delete', { id: environment.id });
      } else {
        await deleteEnvironmentAction(environment.id);
      }
      router.push('/settings/environments');
    } finally {
      setIsDeleting(false);
      setDeleteDialogOpen(false);
    }
  };

  const handleSaveEnvironment = async () => {
    if (!environment) return;
    setIsSaving(true);
    try {
      const payload = {
        name: environment.name,
        kind: environment.kind,
        worktree_root: environment.worktree_root ?? undefined,
        image_tag: environment.image_tag ?? undefined,
        dockerfile: environment.dockerfile ?? undefined,
        build_config: environment.build_config ?? undefined,
      };
      const client = getWebSocketClient();
      const updated = client
        ? await client.request<Environment>('environment.update', { id: environment.id, ...payload })
        : await updateEnvironmentAction(environment.id, payload);
      setEnvironment(updated);
      setSavedEnvironment(updated);
      router.push('/settings/environments');
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <h2 className="text-2xl font-bold">{environment.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            {environment.kind === 'local_pc'
              ? 'Uses your local machine and installed agents.'
              : 'Runs inside a Docker image on the selected executor.'}
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => router.push('/settings/environments')}>
          Back to Environments
        </Button>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle>Environment Name</CardTitle>
        </CardHeader>
        <CardContent>
          <Input
            value={environment.name}
            onChange={(e) => setEnvironment({ ...environment, name: e.target.value })}
            disabled={environment.kind === 'local_pc'}
          />
          {environment.kind === 'local_pc' && (
            <p className="text-xs text-muted-foreground mt-2">
              The local environment name is fixed for now.
            </p>
          )}
        </CardContent>
      </Card>

      {environment.kind === 'local_pc' ? (
        <Card>
          <CardHeader>
            <CardTitle>Local Runtime</CardTitle>
            <CardDescription>Worktrees are created under your local root.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              <Label>Worktree root</Label>
              <Input value={environment.worktree_root ?? '~/kandev'} disabled className="cursor-default" />
            </div>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <IconCube className="h-4 w-4" />
              Docker Image
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label>Base image</Label>
                <Select
                  value={environment.build_config?.base_image ?? 'universal'}
                  onValueChange={(value) =>
                    setEnvironment({
                      ...environment,
                      build_config: {
                        base_image: value,
                        install_agents: environment.build_config?.install_agents ?? '',
                      },
                    })
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.entries(BASE_IMAGE_LABELS).map(([key, label]) => (
                      <SelectItem key={key} value={key}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Image tag</Label>
                <Input
                  value={environment.image_tag ?? ''}
                  onChange={(event) =>
                    setEnvironment({ ...environment, image_tag: event.target.value })
                  }
                />
              </div>
            </div>
            <div className="space-y-2">
              <Label>Dockerfile</Label>
              <Textarea
                value={environment.dockerfile ?? ''}
                onChange={(event) =>
                  setEnvironment({ ...environment, dockerfile: event.target.value })
                }
                rows={10}
                className="font-mono text-sm"
              />
            </div>
          </CardContent>
        </Card>
      )}

      <Separator />

      <div className="flex items-center justify-end gap-2">
        <Button
          variant="outline"
          onClick={() => router.push('/settings/environments')}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSaveEnvironment}
          disabled={!isDirty || isSaving || environment.kind === 'local_pc'}
        >
          {isSaving ? 'Saving...' : 'Save Changes'}
        </Button>
      </div>

      {environment.kind !== 'local_pc' && (
        <>
          <Card className="border-destructive">
            <CardHeader>
              <CardTitle className="text-destructive">Danger Zone</CardTitle>
              <CardDescription>Irreversible actions that permanently delete this environment.</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Delete this environment</p>
                  <p className="text-sm text-muted-foreground">
                    Once deleted, all configuration will be permanently removed.
                  </p>
                </div>
                <Button variant="destructive" onClick={() => setDeleteDialogOpen(true)}>
                  <IconTrash className="h-4 w-4 mr-2" />
                  Delete Environment
                </Button>
              </div>
            </CardContent>
          </Card>

          <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Delete Environment</DialogTitle>
                <DialogDescription>
                  This action cannot be undone. This will permanently delete the environment &quot;
                  {environment.name}&quot; and all its data.
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
                  onClick={handleDeleteEnvironment}
                  disabled={deleteConfirmText !== 'delete'}
                >
                  {isDeleting ? 'Deleting...' : 'Delete Environment'}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </>
      )}
    </div>
  );
}
