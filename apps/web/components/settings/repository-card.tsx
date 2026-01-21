'use client';

import { useState } from 'react';
import { IconEdit, IconGitBranch, IconPlus, IconTrash, IconX } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { Card, CardContent } from '@kandev/ui/card';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Textarea } from '@kandev/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';
import { EditableCard } from '@/components/settings/editable-card';
import type { Repository, RepositoryScript } from '@/lib/types/http';

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

type RepositoryCardProps = {
  repository: RepositoryWithScripts;
  isRepositoryDirty: boolean;
  areScriptsDirty: boolean;
  autoOpen?: boolean;
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
  autoOpen = false,
  onUpdate,
  onAddScript,
  onUpdateScript,
  onDeleteScript,
  onSave,
  onDelete,
}: RepositoryCardProps) {
  const { toast } = useToast();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [isEditing, setIsEditing] = useState(() => autoOpen);
  const saveRequest = useRequest(() => onSave(repository.id));
  const deleteRequest = useRequest(async () => { await onDelete(repository.id); });
  const repositoryName = repository.name ?? '';
  const repositoryLocalPath = repository.local_path ?? '';
  const worktreeBranchPrefix = repository.worktree_branch_prefix ?? '';
  const setupScript = repository.setup_script ?? '';
  const cleanupScript = repository.cleanup_script ?? '';
  const devScript = repository.dev_script ?? '';
  const isDirty = isRepositoryDirty || areScriptsDirty;
  const scriptsCount = repository.scripts.length;
  const hasSetupScript = Boolean(setupScript.trim());
  const hasCleanupScript = Boolean(cleanupScript.trim());
  const hasDevScript = Boolean(devScript.trim());
  const showScriptsSummary = scriptsCount > 0 || hasSetupScript || hasCleanupScript || hasDevScript;
  const scriptsLabel = scriptsCount === 0
    ? 'No custom scripts'
    : `${scriptsCount} custom script${scriptsCount === 1 ? '' : 's'}`;
  const sourceLabel = repository.source_type === 'local' ? 'Local' : 'Remote';
  const subtitle = repository.source_type === 'local'
    ? repositoryLocalPath || 'Local path not set'
    : [repository.provider_owner, repository.provider_name].filter(Boolean).join('/') ||
      repository.provider || 'Remote repository';

  const handleSave = async (close: () => void) => {
    try {
      await saveRequest.run();
      close();
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
      setIsEditing(false);
    } catch (error) {
      toast({
        title: 'Failed to delete repository',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  return (
    <>
      <EditableCard
        isEditing={isEditing}
        historyId={`repo-${repository.id}`}
        onOpen={() => setIsEditing(true)}
        onClose={() => setIsEditing(false)}
        renderEdit={({ close }) => (
          <Card>
            <CardContent className="pt-6">
              <div className="space-y-5">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <IconGitBranch className="h-4 w-4 text-muted-foreground" />
                    <Label className="flex items-center gap-2">
                      <span>Repository</span>
                      {isDirty && <UnsavedChangesBadge />}
                    </Label>
                  </div>
                  <div className="flex items-center gap-2">
                    <UnsavedSaveButton
                      isDirty={isDirty}
                      isLoading={saveRequest.isLoading}
                      status={saveRequest.status}
                      cleanLabel="Close"
                      onClick={isDirty ? () => handleSave(close) : close}
                    />
                  </div>
                </div>

                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Repository Name</Label>
                    <Input
                      value={repositoryName}
                      onChange={(e) => onUpdate(repository.id, { name: e.target.value })}
                      placeholder="my-repo"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>Local Path</Label>
                    <Input
                      value={repositoryLocalPath}
                      onChange={(e) => onUpdate(repository.id, { local_path: e.target.value })}
                      placeholder="/path/to/repository"
                      disabled={repository.source_type !== 'local'}
                    />
                  </div>
                </div>

                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Worktree Branch Prefix</Label>
                    <Input
                      value={worktreeBranchPrefix}
                      onChange={(e) =>
                        onUpdate(repository.id, { worktree_branch_prefix: e.target.value })
                      }
                      placeholder="feature/"
                    />
                    <p className="text-xs text-muted-foreground">
                      Used for new worktree branches. Leave empty to use the default.
                      Branches are generated as {'{prefix}{sanitized-title}-{rand}'}.
                    </p>
                  </div>
                </div>

                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label>Setup Script</Label>
                    <Textarea
                      value={setupScript}
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
                      value={cleanupScript}
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

                <div className="space-y-2">
                  <Label>Dev Script</Label>
                  <Textarea
                    value={devScript}
                    onChange={(e) => onUpdate(repository.id, { dev_script: e.target.value })}
                    placeholder="#!/bin/bash&#10;npm run dev"
                    rows={3}
                    className="font-mono text-sm"
                  />
                  <p className="text-xs text-muted-foreground">
                    Used to start the preview dev server for this repository.
                  </p>
                </div>

                <div className="space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <Label className="flex items-center gap-2">
                      <span>Custom Scripts</span>
                      {areScriptsDirty && <UnsavedChangesBadge />}
                    </Label>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => onAddScript(repository.id)}
                    >
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
                              value={script.name ?? ''}
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
                            value={script.command ?? ''}
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
          </Card>
        )}
        renderPreview={({ open }) => (
          <Card>
            <CardContent className="py-4 cursor-pointer" onClick={open}>
              <div className="flex items-start justify-between gap-3">
                <div className="flex items-start gap-3 min-w-0">
                  <div className="p-2 bg-muted rounded-md">
                    <IconGitBranch className="h-4 w-4 text-muted-foreground" />
                  </div>
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <h4 className="font-medium truncate">
                        {repositoryName || 'Untitled repository'}
                      </h4>
                      <Badge variant="secondary" className="text-xs">
                        {sourceLabel}
                      </Badge>
                      {worktreeBranchPrefix.trim() && worktreeBranchPrefix.trim() !== 'feature/' ? (
                        <Badge variant="outline" className="text-xs">
                          {worktreeBranchPrefix.trim()}
                        </Badge>
                      ) : null}
                      {isDirty && <UnsavedChangesBadge />}
                    </div>
                    <div className="text-xs text-muted-foreground mt-1 truncate">
                      {subtitle}
                    </div>
                    {showScriptsSummary ? (
                      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground mt-1">
                        {scriptsCount > 0 && <span>{scriptsLabel}</span>}
                        {hasSetupScript && <span>build script</span>}
                        {hasCleanupScript && <span>cleanup script</span>}
                        {hasDevScript && <span>dev script</span>}
                      </div>
                    ) : null}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="cursor-pointer"
                    onClick={(event) => {
                      event.stopPropagation();
                      open();
                    }}
                  >
                    <IconEdit className="h-4 w-4" />
                    Edit
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="cursor-pointer"
                    onClick={(event) => {
                      event.stopPropagation();
                      setDeleteOpen(true);
                    }}
                    disabled={deleteRequest.isLoading}
                  >
                    <IconTrash className="h-4 w-4" />
                    Delete
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        )}
      />
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
    </>
  );
}
