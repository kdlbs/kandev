'use client';

import { useState } from 'react';
import { IconGitBranch, IconPlus, IconTrash, IconX } from '@tabler/icons-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';
import type { Repository, RepositoryScript } from '@/lib/types/http';

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

type RepositoryCardProps = {
  repository: RepositoryWithScripts;
  isRepositoryDirty: boolean;
  areScriptsDirty: boolean;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
  onAddScript: (repoId: string) => void;
  onUpdateScript: (repoId: string, scriptId: string, updates: Partial<RepositoryScript>) => void;
  onDeleteScript: (repoId: string, scriptId: string) => void;
  onSave: (repoId: string) => Promise<void>;
  onDelete: (repoId: string) => Promise<void> | void;
};

export function RepositoryCard({
  repository,
  isRepositoryDirty,
  areScriptsDirty,
  onUpdate,
  onAddScript,
  onUpdateScript,
  onDeleteScript,
  onSave,
  onDelete,
}: RepositoryCardProps) {
  const { toast } = useToast();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const saveRequest = useRequest(() => onSave(repository.id));
  const deleteRequest = useRequest(async () => { await onDelete(repository.id); });

  const handleSave = async () => {
    try {
      await saveRequest.run();
    } catch (error) {
      toast({
        title: 'Failed to save repository',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleDelete = async () => {
    try {
      await deleteRequest.run();
      setDeleteOpen(false);
    } catch (error) {
      toast({
        title: 'Failed to delete repository',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="space-y-5">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-2">
              <IconGitBranch className="h-4 w-4 text-muted-foreground" />
              <Label className="flex items-center gap-2">
                <span>Repository</span>
                {isRepositoryDirty && <UnsavedChangesBadge />}
              </Label>
            </div>
            <UnsavedSaveButton
              isDirty={isRepositoryDirty || areScriptsDirty}
              isLoading={saveRequest.isLoading}
              status={saveRequest.status}
              onClick={handleSave}
            />
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label>Repository Name</Label>
              <Input
                value={repository.name}
                onChange={(e) => onUpdate(repository.id, { name: e.target.value })}
                placeholder="my-repo"
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label>Local Path</Label>
            <Input
              value={repository.local_path}
              onChange={(e) => onUpdate(repository.id, { local_path: e.target.value })}
              placeholder="/path/to/repository"
              disabled={repository.source_type !== 'local'}
            />
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label>Setup Script</Label>
              <Textarea
                value={repository.setup_script}
                onChange={(e) => onUpdate(repository.id, { setup_script: e.target.value })}
                placeholder="#!/bin/bash&#10;# any manual setup you need"
                rows={3}
                className="font-mono text-sm"
              />
              <p className="text-xs text-muted-foreground">
                Runs when the repo is cloned or a git worktree is created.
              </p>
            </div>
            <div className="space-y-2">
              <Label>Cleanup Script</Label>
              <Textarea
                value={repository.cleanup_script}
                onChange={(e) => onUpdate(repository.id, { cleanup_script: e.target.value })}
                placeholder="#!/bin/bash&#10;# any manual clean up you need"
                rows={3}
                className="font-mono text-sm"
              />
              <p className="text-xs text-muted-foreground">
                Runs when the task is completed to clean up the workspace.
              </p>
            </div>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between gap-3">
              <Label className="flex items-center gap-2">
                <span>Custom Scripts</span>
                {areScriptsDirty && <UnsavedChangesBadge />}
              </Label>
              <Button type="button" variant="outline" size="sm" onClick={() => onAddScript(repository.id)}>
                <IconPlus className="h-4 w-4 mr-1" />
                Add Script
              </Button>
            </div>
            <div className="space-y-3">
              {repository.scripts.length === 0 ? (
                <p className="text-sm text-muted-foreground">No scripts yet.</p>
              ) : (
                repository.scripts.map((script) => (
                  <div key={script.id} className="grid gap-2">
                    <div className="flex items-center gap-2">
                      <Input
                        value={script.name}
                        onChange={(e) =>
                          onUpdateScript(repository.id, script.id, { name: e.target.value })
                        }
                        placeholder="Script name"
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        onClick={() => onDeleteScript(repository.id, script.id)}
                      >
                        <IconX className="h-4 w-4" />
                      </Button>
                    </div>
                    <Textarea
                      value={script.command}
                      onChange={(e) =>
                        onUpdateScript(repository.id, script.id, { command: e.target.value })
                      }
                      placeholder="#!/bin/bash&#10;npm run dev"
                      rows={3}
                      className="font-mono text-sm"
                    />
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="flex justify-end">
            <Button
              type="button"
              variant="destructive"
              onClick={() => setDeleteOpen(true)}
              disabled={deleteRequest.isLoading}
            >
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Repository
            </Button>
          </div>
        </div>
      </CardContent>
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete repository</DialogTitle>
            <DialogDescription>
              This will remove the repository and its scripts. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteOpen(false)}>
              Cancel
            </Button>
            <Button type="button" variant="destructive" onClick={handleDelete}>
              Delete Repository
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
