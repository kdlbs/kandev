'use client';

import { use, useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconGitBranch, IconFolder, IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SettingsSection } from '@/components/settings/settings-section';
import { RepositoryCard } from '@/components/settings/repository-card';
import { ContextCard } from '@/components/settings/context-card';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import type { Workspace, Repository, Context } from '@/lib/settings/types';

export default function WorkspaceEditPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [workspace, setWorkspace] = useState<Workspace | undefined>(
    SETTINGS_DATA.workspaces.find((w) => w.id === id)
  );
  const [isAddingRepo, setIsAddingRepo] = useState(false);
  const [isAddingContext, setIsAddingContext] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');

  if (!workspace) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Workspace not found</p>
            <Button className="mt-4" onClick={() => router.push('/settings/workspace/new')}>
              Create New Workspace
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const handleUpdateWorkspaceName = (name: string) => {
    setWorkspace({ ...workspace, name });
  };

  const handleAddRepository = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    const newRepo: Repository = {
      id: crypto.randomUUID(),
      name: formData.get('name') as string,
      path: formData.get('path') as string,
      setupScript: formData.get('setupScript') as string,
      cleanupScript: formData.get('cleanupScript') as string,
      customScripts: [],
    };
    setWorkspace({
      ...workspace,
      repositories: [...workspace.repositories, newRepo],
    });
    setIsAddingRepo(false);
  };

  const handleUpdateRepository = (repo: Repository) => {
    setWorkspace({
      ...workspace,
      repositories: workspace.repositories.map((r) => (r.id === repo.id ? repo : r)),
    });
  };

  const handleDeleteRepository = (id: string) => {
    setWorkspace({
      ...workspace,
      repositories: workspace.repositories.filter((r) => r.id !== id),
    });
  };

  const handleAddContext = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    const newContext: Context = {
      id: crypto.randomUUID(),
      name: formData.get('name') as string,
      columns: [
        { id: 'todo', title: 'To Do', color: 'bg-neutral-400' },
        { id: 'in-progress', title: 'In Progress', color: 'bg-blue-500' },
        { id: 'done', title: 'Done', color: 'bg-green-500' },
      ],
    };
    setWorkspace({
      ...workspace,
      contexts: [...workspace.contexts, newContext],
    });
    setIsAddingContext(false);
  };

  const handleUpdateContext = (context: Context) => {
    setWorkspace({
      ...workspace,
      contexts: workspace.contexts.map((c) => (c.id === context.id ? context : c)),
    });
  };

  const handleDeleteContext = (id: string) => {
    setWorkspace({
      ...workspace,
      contexts: workspace.contexts.filter((c) => c.id !== id),
    });
  };

  const handleFolderPick = (callback: (path: string) => void) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.webkitdirectory = true;
    input.onchange = (e: any) => {
      const files = e.target.files;
      if (files && files[0]) {
        const fullPath = files[0].webkitRelativePath;
        const folderName = fullPath.split('/')[0];
        callback(`/${folderName}`);
      }
    };
    input.click();
  };

  const handleDeleteWorkspace = () => {
    if (deleteConfirmText === 'delete') {
      // In real app, this would delete from database
      router.push('/settings/general');
      setDeleteDialogOpen(false);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{workspace.name}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Manage repositories and contexts for this workspace
        </p>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle>Workspace Name</CardTitle>
        </CardHeader>
        <CardContent>
          <Input
            value={workspace.name}
            onChange={(e) => handleUpdateWorkspaceName(e.target.value)}
          />
        </CardContent>
      </Card>

      <Separator />

      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title="Repositories"
        description="Repositories in this workspace"
        action={
          <Button size="sm" onClick={() => setIsAddingRepo(true)}>
            <IconPlus className="h-4 w-4 mr-2" />
            Add Repository
          </Button>
        }
      >
        <div className="grid gap-3">
          {isAddingRepo && (
            <Card>
              <CardContent className="pt-6">
                <form onSubmit={handleAddRepository} className="space-y-4">
                  <div className="space-y-2">
                    <Label>Repository Name</Label>
                    <Input name="name" placeholder="my-project" required />
                  </div>
                  <div className="space-y-2">
                    <Label>Directory Path</Label>
                    <div className="flex gap-2">
                      <Input
                        id="repo-path"
                        name="path"
                        placeholder="/path/to/repository"
                        required
                      />
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() =>
                          handleFolderPick((path) => {
                            const input = document.getElementById('repo-path') as HTMLInputElement;
                            if (input) input.value = path;
                          })
                        }
                      >
                        Browse
                      </Button>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Label>Setup Script</Label>
                    <Textarea
                      name="setupScript"
                      placeholder="#!/bin/bash&#10;npm install"
                      rows={3}
                      className="font-mono text-sm"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label>Cleanup Script</Label>
                    <Textarea
                      name="cleanupScript"
                      placeholder="#!/bin/bash&#10;rm -rf node_modules"
                      rows={3}
                      className="font-mono text-sm"
                    />
                  </div>
                  <div className="flex gap-2 justify-end">
                    <Button type="button" variant="outline" onClick={() => setIsAddingRepo(false)}>
                      Cancel
                    </Button>
                    <Button type="submit">Add</Button>
                  </div>
                </form>
              </CardContent>
            </Card>
          )}

          {workspace.repositories.map((repo) => (
            <RepositoryCard
              key={repo.id}
              repository={repo}
              onUpdate={handleUpdateRepository}
              onDelete={() => handleDeleteRepository(repo.id)}
            />
          ))}

          {workspace.repositories.length === 0 && !isAddingRepo && (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-sm text-muted-foreground">No repositories in this workspace</p>
              </CardContent>
            </Card>
          )}
        </div>
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconFolder className="h-5 w-5" />}
        title="Contexts"
        description="Kanban view contexts for this workspace"
        action={
          <Button size="sm" onClick={() => setIsAddingContext(true)}>
            <IconPlus className="h-4 w-4 mr-2" />
            Add Context
          </Button>
        }
      >
        <div className="grid gap-3">
          {isAddingContext && (
            <Card>
              <CardContent className="pt-6">
                <form onSubmit={handleAddContext} className="space-y-4">
                  <div className="space-y-2">
                    <Label>Context Name</Label>
                    <Input name="name" placeholder="Dev" required />
                  </div>
                  <div className="flex gap-2 justify-end">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setIsAddingContext(false)}
                    >
                      Cancel
                    </Button>
                    <Button type="submit">Add</Button>
                  </div>
                </form>
              </CardContent>
            </Card>
          )}

          {workspace.contexts.map((context) => (
            <ContextCard
              key={context.id}
              context={context}
              onUpdate={handleUpdateContext}
              onDelete={() => handleDeleteContext(context.id)}
            />
          ))}

          {workspace.contexts.length === 0 && !isAddingContext && (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-sm text-muted-foreground">No contexts in this workspace</p>
              </CardContent>
            </Card>
          )}
        </div>
      </SettingsSection>

      <Separator />

      {/* Danger Zone */}
      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">Danger Zone</CardTitle>
          <CardDescription>
            Irreversible actions that will permanently delete this workspace
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="font-medium">Delete this workspace</p>
              <p className="text-sm text-muted-foreground">
                Once deleted, all repositories and contexts will be permanently removed
              </p>
            </div>
            <Button variant="destructive" onClick={() => setDeleteDialogOpen(true)}>
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Workspace
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Workspace</DialogTitle>
            <DialogDescription>
              This action cannot be undone. This will permanently delete the workspace &quot;
              {workspace.name}&quot; and all its data.
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
              onClick={handleDeleteWorkspace}
              disabled={deleteConfirmText !== 'delete'}
            >
              Delete Workspace
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
