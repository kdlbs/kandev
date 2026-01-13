'use client';

import { useEffect, useState } from 'react';
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
import { listExecutorsAction } from '@/app/actions/executors';
import { listEnvironmentsAction } from '@/app/actions/environments';
import { listAgentsAction } from '@/app/actions/agents';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { Agent, Environment, Executor, Workspace } from '@/lib/types/http';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { useAppStore } from '@/components/state-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';

type WorkspaceEditClientProps = {
  workspace: Workspace | null;
};

type AgentProfileOption = {
  id: string;
  label: string;
};

export function WorkspaceEditClient({ workspace }: WorkspaceEditClientProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace | null>(workspace);
  const [workspaceNameDraft, setWorkspaceNameDraft] = useState(workspace?.name ?? '');
  const [savedWorkspaceName, setSavedWorkspaceName] = useState(workspace?.name ?? '');
  const [savedDefaultExecutorId, setSavedDefaultExecutorId] = useState(
    workspace?.default_executor_id ?? ''
  );
  const [savedDefaultEnvironmentId, setSavedDefaultEnvironmentId] = useState(
    workspace?.default_environment_id ?? ''
  );
  const [savedDefaultAgentProfileId, setSavedDefaultAgentProfileId] = useState(
    workspace?.default_agent_profile_id ?? ''
  );
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [defaultExecutorId, setDefaultExecutorId] = useState(
    workspace?.default_executor_id ?? ''
  );
  const [defaultEnvironmentId, setDefaultEnvironmentId] = useState(
    workspace?.default_environment_id ?? ''
  );
  const [defaultAgentProfileId, setDefaultAgentProfileId] = useState(
    workspace?.default_agent_profile_id ?? ''
  );
  const [executors, setExecutors] = useState<Executor[]>([]);
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [agentProfiles, setAgentProfiles] = useState<AgentProfileOption[]>([]);
  const [agentProfilesLoading, setAgentProfilesLoading] = useState(true);
  const [environmentsLoading, setEnvironmentsLoading] = useState(true);
  const saveWorkspaceRequest = useRequest(updateWorkspaceAction);
  const deleteWorkspaceRequest = useRequest(deleteWorkspaceAction);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);
  const activeExecutors = executors.filter((executor) => executor.status === 'active');
  const isDefaultExecutorDirty = defaultExecutorId !== savedDefaultExecutorId;
  const isDefaultEnvironmentDirty = defaultEnvironmentId !== savedDefaultEnvironmentId;
  const isDefaultAgentProfileDirty = defaultAgentProfileId !== savedDefaultAgentProfileId;

  useEffect(() => {
    const client = getWebSocketClient();
    if (client) {
      client
        .request<{ executors: Executor[] }>('executor.list', {})
        .then((resp) => setExecutors(resp.executors))
        .catch(() => setExecutors([]));
      return;
    }
    listExecutorsAction()
      .then((resp) => setExecutors(resp.executors))
      .catch(() => setExecutors([]));
  }, []);

  useEffect(() => {
    const client = getWebSocketClient();
    if (client) {
      client
        .request<{ environments: Environment[] }>('environment.list', {})
        .then((resp) => setEnvironments(resp.environments))
        .catch(() => setEnvironments([]))
        .finally(() => setEnvironmentsLoading(false));
      return;
    }
    listEnvironmentsAction()
      .then((resp) => setEnvironments(resp.environments))
      .catch(() => setEnvironments([]))
      .finally(() => setEnvironmentsLoading(false));
  }, []);

  useEffect(() => {
    listAgentsAction()
      .then((resp) => {
        const options = resp.agents.flatMap((agent: Agent) =>
          agent.profiles.map((profile) => ({
            id: profile.id,
            label: `${agent.name} • ${profile.name}`,
          }))
        );
        setAgentProfiles(options);
      })
      .catch(() => setAgentProfiles([]))
      .finally(() => setAgentProfilesLoading(false));
  }, []);

  const handleSaveWorkspaceName = async () => {
    if (!currentWorkspace) return;
    const trimmed = workspaceNameDraft.trim();
    if (!trimmed || trimmed === currentWorkspace.name) return;
    try {
      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, { name: trimmed });
      setCurrentWorkspace((prev) => (prev ? { ...prev, ...updated } : prev));
      setSavedWorkspaceName(updated.name);
      setWorkspaces(
        workspaces.map((workspace) =>
          workspace.id === updated.id
            ? {
                ...workspace,
                name: updated.name,
                default_executor_id: updated.default_executor_id ?? null,
                default_environment_id: updated.default_environment_id ?? null,
                default_agent_profile_id: updated.default_agent_profile_id ?? null,
              }
            : workspace
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
    if (deleteConfirmText !== 'delete' || !currentWorkspace) return;
    try {
      await deleteWorkspaceRequest.run(currentWorkspace.id);
      setWorkspaces(workspaces.filter((workspace) => workspace.id !== currentWorkspace.id));
      router.push('/settings/workspace');
    } catch (error) {
      toast({
        title: 'Failed to delete workspace',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleSaveDefaultExecutor = async () => {
    if (!currentWorkspace) return;
    if (defaultExecutorId === savedDefaultExecutorId) return;
    try {
      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, {
        default_executor_id: defaultExecutorId,
      });
      setCurrentWorkspace((prev) => (prev ? { ...prev, ...updated } : prev));
      setSavedDefaultExecutorId(updated.default_executor_id ?? '');
      setWorkspaces(
        workspaces.map((workspace) =>
          workspace.id === updated.id
            ? {
                ...workspace,
                default_executor_id: updated.default_executor_id ?? null,
                default_environment_id: updated.default_environment_id ?? null,
                default_agent_profile_id: updated.default_agent_profile_id ?? null,
              }
            : workspace
        )
      );
    } catch (error) {
      toast({
        title: 'Failed to save executor',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleSaveDefaultEnvironment = async () => {
    if (!currentWorkspace) return;
    if (defaultEnvironmentId === savedDefaultEnvironmentId) return;
    try {
      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, {
        default_environment_id: defaultEnvironmentId,
      });
      setCurrentWorkspace((prev) => (prev ? { ...prev, ...updated } : prev));
      setSavedDefaultEnvironmentId(updated.default_environment_id ?? '');
      setWorkspaces(
        workspaces.map((workspace) =>
          workspace.id === updated.id
            ? {
                ...workspace,
                default_executor_id: updated.default_executor_id ?? null,
                default_environment_id: updated.default_environment_id ?? null,
                default_agent_profile_id: updated.default_agent_profile_id ?? null,
              }
            : workspace
        )
      );
    } catch (error) {
      toast({
        title: 'Failed to save environment',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleSaveDefaultAgentProfile = async () => {
    if (!currentWorkspace) return;
    if (defaultAgentProfileId === savedDefaultAgentProfileId) return;
    try {
      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, {
        default_agent_profile_id: defaultAgentProfileId,
      });
      setCurrentWorkspace((prev) => (prev ? { ...prev, ...updated } : prev));
      setSavedDefaultAgentProfileId(updated.default_agent_profile_id ?? '');
      setWorkspaces(
        workspaces.map((workspace) =>
          workspace.id === updated.id
            ? {
                ...workspace,
                default_executor_id: updated.default_executor_id ?? null,
                default_environment_id: updated.default_environment_id ?? null,
                default_agent_profile_id: updated.default_agent_profile_id ?? null,
              }
            : workspace
        )
      );
    } catch (error) {
      toast({
        title: 'Failed to save agent profile',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  if (!currentWorkspace) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Workspace not found</p>
            <Button className="mt-4" onClick={() => router.push('/settings/workspace')}>
              Back to Workspaces
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{currentWorkspace.name}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Manage workspace details and jump into boards or repositories.
        </p>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span>Workspace Name</span>
            {workspaceNameDraft.trim() !== savedWorkspaceName && <UnsavedChangesBadge />}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <Input
              value={workspaceNameDraft}
              onChange={(e) => setWorkspaceNameDraft(e.target.value)}
            />
            <div className="flex items-center gap-2">
              <UnsavedSaveButton
                isDirty={workspaceNameDraft.trim() !== savedWorkspaceName}
                isLoading={saveWorkspaceRequest.isLoading}
                status={saveWorkspaceRequest.status}
                onClick={handleSaveWorkspaceName}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span>Default Executor</span>
            {isDefaultExecutorDirty && <UnsavedChangesBadge />}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Label htmlFor="default-executor">Executor</Label>
          <Select
            value={defaultExecutorId || 'none'}
            onValueChange={(value) => setDefaultExecutorId(value === 'none' ? '' : value)}
          >
            <SelectTrigger id="default-executor">
              <SelectValue placeholder="Select executor" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">No default</SelectItem>
              {activeExecutors.map((executor) => (
                <SelectItem key={executor.id} value={executor.id}>
                  {executor.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex items-center gap-2">
            <UnsavedSaveButton
              isDirty={isDefaultExecutorDirty}
              isLoading={saveWorkspaceRequest.isLoading}
              status={saveWorkspaceRequest.status}
              onClick={handleSaveDefaultExecutor}
            />
            <p className="text-xs text-muted-foreground">
              Select which executor new sessions should default to.
            </p>
          </div>
        </CardContent>
      </Card>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span>Default Environment</span>
            {isDefaultEnvironmentDirty && <UnsavedChangesBadge />}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Label htmlFor="default-environment">Environment</Label>
          <Select
            value={defaultEnvironmentId || 'none'}
            onValueChange={(value) => setDefaultEnvironmentId(value === 'none' ? '' : value)}
            disabled={environmentsLoading}
          >
            <SelectTrigger id="default-environment">
              <SelectValue
                placeholder={environmentsLoading ? 'Loading environments...' : 'Select environment'}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">No default</SelectItem>
              {environments.map((environment) => (
                <SelectItem key={environment.id} value={environment.id}>
                  {environment.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex items-center gap-2">
            <UnsavedSaveButton
              isDirty={isDefaultEnvironmentDirty}
              isLoading={saveWorkspaceRequest.isLoading}
              status={saveWorkspaceRequest.status}
              onClick={handleSaveDefaultEnvironment}
            />
            <p className="text-xs text-muted-foreground">
              Select which environment new tasks should default to.
            </p>
          </div>
        </CardContent>
      </Card>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span>Default Agent Profile</span>
            {isDefaultAgentProfileDirty && <UnsavedChangesBadge />}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Label htmlFor="default-agent-profile">Agent profile</Label>
          <Select
            value={defaultAgentProfileId || 'none'}
            onValueChange={(value) => setDefaultAgentProfileId(value === 'none' ? '' : value)}
            disabled={agentProfilesLoading}
          >
            <SelectTrigger id="default-agent-profile">
              <SelectValue
                placeholder={agentProfilesLoading ? 'Loading agent profiles...' : 'Select agent profile'}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">No default</SelectItem>
              {agentProfiles.map((profile) => (
                <SelectItem key={profile.id} value={profile.id}>
                  {profile.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex items-center gap-2">
            <UnsavedSaveButton
              isDirty={isDefaultAgentProfileDirty}
              isLoading={saveWorkspaceRequest.isLoading}
              status={saveWorkspaceRequest.status}
              onClick={handleSaveDefaultAgentProfile}
            />
            <p className="text-xs text-muted-foreground">
              Select which agent profile new tasks should default to.
            </p>
          </div>
        </CardContent>
      </Card>

      <Separator />

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
              <Link href={`/settings/workspace/${currentWorkspace.id}/boards`}>
                <IconLayoutColumns className="h-4 w-4" />
                Boards
              </Link>
            </Button>
          </div>
        </CardContent>
      </Card>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="text-destructive">Delete Workspace</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              Deleting this workspace will remove all boards and tasks associated with it.
            </p>
            <Button variant="destructive" onClick={() => setDeleteDialogOpen(true)}>
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Workspace
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Workspace</DialogTitle>
            <DialogDescription>
              This will delete all boards and tasks. This action cannot be undone. Type &quot;delete&quot; to confirm.
            </DialogDescription>
          </DialogHeader>
          <Input
            value={deleteConfirmText}
            onChange={(e) => setDeleteConfirmText(e.target.value)}
            placeholder="delete"
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDeleteWorkspace}>
              Delete Workspace
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
