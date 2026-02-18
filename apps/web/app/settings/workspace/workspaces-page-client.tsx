'use client';

import { useState } from 'react';
import Link from 'next/link';
import { IconFolder, IconPlus, IconChevronRight } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { createWorkspaceAction } from '@/app/actions/workspaces';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { RequestIndicator } from '@/components/request-indicator';
import { useAppStore } from '@/components/state-provider';
import type { Workspace } from '@/lib/types/http';

type AddWorkspaceFormProps = {
  newWorkspaceName: string;
  onNameChange: (value: string) => void;
  onSubmit: (e: React.FormEvent) => void;
  onCancel: () => void;
  isLoading: boolean;
  status: 'idle' | 'loading' | 'success' | 'error';
};

function AddWorkspaceForm({ newWorkspaceName, onNameChange, onSubmit, onCancel, isLoading, status }: AddWorkspaceFormProps) {
  return (
    <Card>
      <CardContent className="pt-6">
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="workspace-name">Workspace Name</Label>
            <Input
              id="workspace-name"
              value={newWorkspaceName}
              onChange={(e) => onNameChange(e.target.value)}
              placeholder="My Workspace"
              required
              autoFocus
            />
          </div>
          <div className="flex gap-2 justify-end">
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
            <div className="flex items-center gap-2">
              <RequestIndicator status={status} />
              <Button type="submit" disabled={isLoading}>
                Add Workspace
              </Button>
            </div>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function WorkspaceListItem({ workspace }: { workspace: Workspace }) {
  return (
    <Link href={`/settings/workspace/${workspace.id}`}>
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
                  <span>Workflows managed in this workspace</span>
                </div>
              </div>
            </div>
            <IconChevronRight className="h-5 w-5 text-muted-foreground" />
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

export function WorkspacesPageClient() {
  const items = useAppStore((state) => state.workspaces.items);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);
  const [isAdding, setIsAdding] = useState(false);
  const [newWorkspaceName, setNewWorkspaceName] = useState('');
  const createRequest = useRequest(createWorkspaceAction);
  const { toast } = useToast();

  const handleAddWorkspace = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newWorkspaceName.trim()) return;
    try {
      const created = await createRequest.run({ name: newWorkspaceName.trim() });
      setWorkspaces([
        {
          id: created.id,
          name: created.name,
          description: created.description ?? null,
          owner_id: created.owner_id,
          default_executor_id: created.default_executor_id ?? null,
          default_environment_id: created.default_environment_id ?? null,
          default_agent_profile_id: created.default_agent_profile_id ?? null,
          created_at: created.created_at,
          updated_at: created.updated_at,
        },
        ...items.map((workspace: Workspace) => ({
          id: workspace.id,
          name: workspace.name,
          description: workspace.description ?? null,
          owner_id: workspace.owner_id,
          default_executor_id: workspace.default_executor_id ?? null,
          default_environment_id: workspace.default_environment_id ?? null,
          default_agent_profile_id: workspace.default_agent_profile_id ?? null,
          created_at: workspace.created_at,
          updated_at: workspace.updated_at,
        })),
      ]);
      setNewWorkspaceName('');
      setIsAdding(false);
    } catch (error) {
      toast({
        title: 'Failed to create workspace',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold">Workspaces</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Manage your workspaces and workflows
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
            <AddWorkspaceForm
              newWorkspaceName={newWorkspaceName}
              onNameChange={setNewWorkspaceName}
              onSubmit={handleAddWorkspace}
              onCancel={() => setIsAdding(false)}
              isLoading={createRequest.isLoading}
              status={createRequest.status}
            />
          )}

          {items.map((workspace: Workspace) => (
            <WorkspaceListItem key={workspace.id} workspace={workspace} />
          ))}

          {items.length === 0 && (
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
