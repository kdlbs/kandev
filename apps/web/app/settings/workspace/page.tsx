'use client';

import { useState } from 'react';
import Link from 'next/link';
import { IconFolder, IconPlus, IconChevronRight } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { SETTINGS_DATA } from '@/lib/settings/dummy-data';
import type { Workspace } from '@/lib/settings/types';

export default function WorkspacesPage() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>(SETTINGS_DATA.workspaces);
  const [isAdding, setIsAdding] = useState(false);
  const [newWorkspaceName, setNewWorkspaceName] = useState('');

  const handleAddWorkspace = (e: React.FormEvent) => {
    e.preventDefault();
    if (newWorkspaceName.trim()) {
      const newWorkspace: Workspace = {
        id: crypto.randomUUID(),
        name: newWorkspaceName,
        repositories: [],
        contexts: [],
      };
      setWorkspaces([...workspaces, newWorkspace]);
      setNewWorkspaceName('');
      setIsAdding(false);
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold">Workspaces</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Manage your workspaces with repositories and contexts
          </p>
        </div>
        <Button size="sm" onClick={() => setIsAdding(true)}>
          <IconPlus className="h-4 w-4 mr-2" />
          Add Workspace
        </Button>
      </div>

      <Separator />

      <div className="space-y-4">
        <div className="grid gap-3">
          {isAdding && (
            <Card>
              <CardContent className="pt-6">
                <form onSubmit={handleAddWorkspace} className="space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="workspace-name">Workspace Name</Label>
                    <Input
                      id="workspace-name"
                      value={newWorkspaceName}
                      onChange={(e) => setNewWorkspaceName(e.target.value)}
                      placeholder="My Workspace"
                      required
                      autoFocus
                    />
                  </div>
                  <div className="flex gap-2 justify-end">
                    <Button type="button" variant="outline" onClick={() => setIsAdding(false)}>
                      Cancel
                    </Button>
                    <Button type="submit">Add Workspace</Button>
                  </div>
                </form>
              </CardContent>
            </Card>
          )}

          {workspaces.map((workspace) => (
            <Link key={workspace.id} href={`/settings/workspace/${workspace.id}`}>
              <Card className="hover:bg-accent transition-colors cursor-pointer">
                <CardContent className="py-4">
                  <div className="flex items-start justify-between">
                    <div className="flex items-start gap-3 flex-1">
                      <div className="p-2 bg-muted rounded-md">
                        <IconFolder className="h-4 w-4" />
                      </div>
                      <div className="flex-1 min-w-0">
                        <h4 className="font-medium">{workspace.name}</h4>
                        <div className="flex items-center gap-3 text-xs text-muted-foreground mt-1">
                          <span>{workspace.repositories.length} repositories</span>
                          <span>{workspace.contexts.length} contexts</span>
                        </div>
                      </div>
                    </div>
                    <IconChevronRight className="h-5 w-5 text-muted-foreground" />
                  </div>
                </CardContent>
              </Card>
            </Link>
          ))}

          {workspaces.length === 0 && (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-sm text-muted-foreground">
                  No workspaces configured. Add your first workspace to get started.
                </p>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
