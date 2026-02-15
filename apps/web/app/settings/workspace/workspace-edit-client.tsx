'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { IconGitBranch, IconLayoutColumns, IconTrash } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import { updateWorkspaceAction, deleteWorkspaceAction } from '@/app/actions/workspaces';
import type { Workspace, Executor, Environment } from '@/lib/types/http';
import type { AgentProfileOption } from '@/lib/state/slices';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { useAppStore } from '@/components/state-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';

type WorkspaceEditClientProps = {
  workspaceId: string;
};

export function WorkspaceEditClient({ workspaceId }: WorkspaceEditClientProps) {
  const workspace = useAppStore((state) =>
    state.workspaces.items.find((item: Workspace) => item.id === workspaceId) ?? null
  );

  if (!workspace) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Workspace not found</p>
            <Button className="mt-4" asChild>
              <Link href="/settings/workspace">Back to Workspaces</Link>
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <WorkspaceEditForm key={workspace.id} workspace={workspace} />;
}

type WorkspaceEditFormProps = {
  workspace: Workspace;
};

function WorkspaceEditForm({ workspace }: WorkspaceEditFormProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace>(workspace);

  // Draft state for all fields
  const [workspaceNameDraft, setWorkspaceNameDraft] = useState(workspace.name ?? '');
  const [defaultExecutorId, setDefaultExecutorId] = useState(workspace.default_executor_id ?? '');
  const [defaultEnvironmentId, setDefaultEnvironmentId] = useState(workspace.default_environment_id ?? '');
  const [defaultAgentProfileId, setDefaultAgentProfileId] = useState(workspace.default_agent_profile_id ?? '');

  // Saved state to track dirty
  const [savedState, setSavedState] = useState({
    name: workspace.name ?? '',
    executorId: workspace.default_executor_id ?? '',
    environmentId: workspace.default_environment_id ?? '',
    agentProfileId: workspace.default_agent_profile_id ?? '',
  });

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');

  const executors = useAppStore((state) => state.executors.items);
  const environments = useAppStore((state) => state.environments.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);

  const saveWorkspaceRequest = useRequest(updateWorkspaceAction);
  const deleteWorkspaceRequest = useRequest(deleteWorkspaceAction);

  const activeExecutors = executors.filter((executor: Executor) => executor.status === 'active');

  const isDirty =
    workspaceNameDraft.trim() !== savedState.name ||
    defaultExecutorId !== savedState.executorId ||
    defaultEnvironmentId !== savedState.environmentId ||
    defaultAgentProfileId !== savedState.agentProfileId;

  const handleSave = async () => {
    if (!isDirty) return;
    try {
      const updates: Record<string, string | undefined> = {};
      if (workspaceNameDraft.trim() !== savedState.name) {
        updates.name = workspaceNameDraft.trim();
      }
      if (defaultExecutorId !== savedState.executorId) {
        updates.default_executor_id = defaultExecutorId;
      }
      if (defaultEnvironmentId !== savedState.environmentId) {
        updates.default_environment_id = defaultEnvironmentId;
      }
      if (defaultAgentProfileId !== savedState.agentProfileId) {
        updates.default_agent_profile_id = defaultAgentProfileId;
      }

      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, updates);
      setCurrentWorkspace((prev) => ({ ...prev, ...updated }));
      setSavedState({
        name: updated.name ?? workspaceNameDraft.trim(),
        executorId: updated.default_executor_id ?? '',
        environmentId: updated.default_environment_id ?? '',
        agentProfileId: updated.default_agent_profile_id ?? '',
      });
      setWorkspaces(
        workspaces.map((workspaceItem: Workspace) =>
          workspaceItem.id === updated.id
            ? {
                ...workspaceItem,
                name: updated.name,
                default_executor_id: updated.default_executor_id ?? null,
                default_environment_id: updated.default_environment_id ?? null,
                default_agent_profile_id: updated.default_agent_profile_id ?? null,
              }
            : workspaceItem
        )
      );
    } catch (error) {
      toast({
        title: 'Failed to save workspace',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleDeleteWorkspace = async () => {
    if (deleteConfirmText !== 'delete') return;
    try {
      await deleteWorkspaceRequest.run(currentWorkspace.id);
      setWorkspaces(workspaces.filter((workspaceItem: Workspace) => workspaceItem.id !== currentWorkspace.id));
      router.push('/settings/workspace');
    } catch (error) {
      toast({
        title: 'Failed to delete workspace',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{currentWorkspace.name}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Manage workspace details and jump into workflows or repositories.
        </p>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span>Workspace Settings</span>
            {isDirty && <UnsavedChangesBadge />}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="workspace-name">Name</Label>
              <Input
                id="workspace-name"
                value={workspaceNameDraft}
                onChange={(event) => setWorkspaceNameDraft(event.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label>Default Executor</Label>
              <Select
                value={defaultExecutorId || 'none'}
                onValueChange={(value) => setDefaultExecutorId(value === 'none' ? '' : value)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Select executor" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">No default</SelectItem>
                  {activeExecutors.map((executor: Executor) => (
                    <SelectItem key={executor.id} value={executor.id}>
                      {executor.name}
                    </SelectItem>
                  ))}
                  {executors.length === 0 && (
                    <SelectItem value="" disabled>
                      No executors available
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Default Environment</Label>
              <Select
                value={defaultEnvironmentId || 'none'}
                onValueChange={(value) => setDefaultEnvironmentId(value === 'none' ? '' : value)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Select environment" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">No default</SelectItem>
                  {environments.map((environment: Environment) => (
                    <SelectItem key={environment.id} value={environment.id}>
                      {environment.name}
                    </SelectItem>
                  ))}
                  {environments.length === 0 && (
                    <SelectItem value="empty-environments" disabled>
                      No environments available
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Default Agent Profile</Label>
              <Select
                value={defaultAgentProfileId || 'none'}
                onValueChange={(value) => setDefaultAgentProfileId(value === 'none' ? '' : value)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Select agent profile" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">No default</SelectItem>
                  {agentProfiles.map((profile: AgentProfileOption) => (
                    <SelectItem key={profile.id} value={profile.id}>
                      {profile.label}
                    </SelectItem>
                  ))}
                  {agentProfiles.length === 0 && (
                    <SelectItem value="empty-agent-profiles" disabled>
                      No agent profiles available
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>

            <div className="flex justify-end pt-2">
              <UnsavedSaveButton
                isDirty={isDirty}
                isLoading={saveWorkspaceRequest.isLoading}
                status={saveWorkspaceRequest.status}
                onClick={handleSave}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Workspace Links</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 sm:grid-cols-2">
            <Button asChild variant="outline" className="justify-start gap-2">
              <Link href={`/settings/workspace/${currentWorkspace.id}/repositories`}>
                <IconGitBranch className="h-4 w-4" />
                Repositories
              </Link>
            </Button>
            <Button asChild variant="outline" className="justify-start gap-2">
              <Link href={`/settings/workspace/${currentWorkspace.id}/workflows`}>
                <IconLayoutColumns className="h-4 w-4" />
                Workflows
              </Link>
            </Button>
          </div>
        </CardContent>
      </Card>

      <Separator />

      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">Delete Workspace</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Delete this workspace</p>
              <p className="text-xs text-muted-foreground">This action cannot be undone.</p>
            </div>
            <Button variant="destructive" onClick={() => setDeleteDialogOpen(true)} className="cursor-pointer">
              <IconTrash className="h-4 w-4 mr-2" />
              Delete
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Workspace</DialogTitle>
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
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)} className="cursor-pointer">
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteWorkspace}
              disabled={deleteConfirmText !== 'delete'}
              className="cursor-pointer"
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
