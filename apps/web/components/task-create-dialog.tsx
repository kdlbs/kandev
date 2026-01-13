'use client';

import { useEffect, useRef, useState, FormEvent } from 'react';
import Link from 'next/link';
import { IconSettings } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from '@kandev/ui/dialog';
import { Label } from '@kandev/ui/label';
import { Input } from '@kandev/ui/input';
import { Textarea } from '@kandev/ui/textarea';
import { Button } from '@kandev/ui/button';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import { Combobox } from './combobox';
import { Badge } from '@kandev/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@kandev/ui/tooltip';
import type { Repository, Task, Branch } from '@/lib/types/http';
import { getBackendConfig } from '@/lib/config';
import {
  DEFAULT_LOCAL_ENVIRONMENT_KIND,
  DEFAULT_LOCAL_EXECUTOR_TYPE,
  formatUserHomePath,
  selectPreferredBranch,
  truncateRepoPath,
} from '@/lib/utils';
import { createTask, listRepositories, listRepositoryBranches, updateTask } from '@/lib/http/client';
import { useAppStore } from '@/components/state-provider';

type AgentProfileOption = {
  id: string;
  label: string;
};

type EnvironmentOption = {
  id: string;
  name: string;
  kind: string;
};

type ExecutorOption = {
  id: string;
  name: string;
  type: string;
};


interface TaskCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  boardId: string | null;
  defaultColumnId: string | null;
  columns: Array<{ id: string; title: string }>;
  editingTask?: { id: string; title: string; description?: string; columnId: string; state?: Task['state'] } | null;
  onSuccess?: (task: Task, mode: 'create' | 'edit') => void;
  initialValues?: {
    title: string;
    description?: string;
    repositoryId?: string;
    branch?: string;
    state?: Task['state'];
  };
  submitLabel?: string;
}

export function TaskCreateDialog({
  open,
  onOpenChange,
  workspaceId,
  boardId,
  defaultColumnId,
  columns,
  editingTask,
  onSuccess,
  initialValues,
  submitLabel = 'Create',
}: TaskCreateDialogProps) {
  const isEditMode = submitLabel !== 'Create';
  const [title, setTitle] = useState(initialValues?.title ?? '');
  const [description, setDescription] = useState(initialValues?.description ?? '');
  const [repositoryId, setRepositoryId] = useState(initialValues?.repositoryId ?? '');
  const [branch, setBranch] = useState(initialValues?.branch ?? '');
  const [startAgent, setStartAgent] = useState(false);
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [repositoriesLoading, setRepositoriesLoading] = useState(false);
  const [branchesByRepo, setBranchesByRepo] = useState<Record<string, Branch[]>>({});
  const [branchesLoading, setBranchesLoading] = useState(false);
  const [agentProfileId, setAgentProfileId] = useState('');
  const [agentProfiles, setAgentProfiles] = useState<AgentProfileOption[]>([]);
  const [agentProfilesLoading, setAgentProfilesLoading] = useState(false);
  const [environmentId, setEnvironmentId] = useState('');
  const [environments, setEnvironments] = useState<EnvironmentOption[]>([]);
  const [environmentsLoading, setEnvironmentsLoading] = useState(false);
  const [executorId, setExecutorId] = useState('');
  const [executors, setExecutors] = useState<ExecutorOption[]>([]);
  const [executorsLoading, setExecutorsLoading] = useState(false);
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    if (!open) return;
    setTitle(initialValues?.title ?? '');
    setDescription(initialValues?.description ?? '');
    setRepositoryId(initialValues?.repositoryId ?? '');
    setBranch(initialValues?.branch ?? '');
    setStartAgent(false);
    setAgentProfileId('');
    setEnvironmentId('');
    setExecutorId('');
  }, [
    initialValues?.branch,
    initialValues?.description,
    initialValues?.repositoryId,
    initialValues?.title,
    open,
  ]);

  const workspaceDefaults = workspaceId
    ? workspaces.find((workspace) => workspace.id === workspaceId)
    : null;

  useEffect(() => {
    if (!open || !workspaceId) return;
    setRepositoriesLoading(true);
    listRepositories(getBackendConfig().apiBaseUrl, workspaceId)
      .then((response) => {
        setRepositories(response.repositories);
        // Auto-select if only one repository
        if (response.repositories.length === 1) {
          setRepositoryId(response.repositories[0].id);
        }
      })
      .catch(() => {
        setRepositories([]);
      })
      .finally(() => {
        setRepositoriesLoading(false);
      });
  }, [open, workspaceId]);

  useEffect(() => {
    if (!repositoryId) return;
    if (branchesByRepo[repositoryId]) {
      if (!branch) {
        const preferredBranch = selectPreferredBranch(branchesByRepo[repositoryId]);
        if (preferredBranch) {
          setBranch(preferredBranch);
          return;
        }
      }
      return;
    }
    setBranchesLoading(true);
    listRepositoryBranches(getBackendConfig().apiBaseUrl, repositoryId)
      .then((response) => {
        setBranchesByRepo((prev) => ({ ...prev, [repositoryId]: response.branches }));

        if (!branch) {
          const preferredBranch = selectPreferredBranch(response.branches);
          if (preferredBranch) {
            setBranch(preferredBranch);
            return;
          }
        }
      })
      .catch(() => {
        setBranchesByRepo((prev) => ({ ...prev, [repositoryId]: [] }));
      })
      .finally(() => {
        setBranchesLoading(false);
      });
  }, [branchesByRepo, repositoryId, branch]);

  useEffect(() => {
    if (!open) return;
    setAgentProfilesLoading(true);
    fetch(`${getBackendConfig().apiBaseUrl}/api/v1/agents`, { cache: 'no-store' })
      .then((response) => (response.ok ? response.json() : null))
      .then((data) => {
        if (!data?.agents) {
          setAgentProfiles([]);
          return;
        }
        const options = data.agents.flatMap(
          (agent: { name: string; profiles: Array<{ id: string; name: string }> }) =>
            agent.profiles.map((profile) => ({
              id: profile.id,
              label: `${agent.name} • ${profile.name}`,
            }))
        );
        setAgentProfiles(options);
      })
      .catch(() => {
        setAgentProfiles([]);
      })
      .finally(() => {
        setAgentProfilesLoading(false);
      });
  }, [open]);

  useEffect(() => {
    if (!open || agentProfileId || agentProfiles.length === 0) return;
    if (isEditMode && agentProfiles.length > 1) return;
    const defaultProfileId = workspaceDefaults?.default_agent_profile_id ?? null;
    if (defaultProfileId && agentProfiles.some((profile) => profile.id === defaultProfileId)) {
      setAgentProfileId(defaultProfileId);
      return;
    }
    setAgentProfileId(agentProfiles[0].id);
  }, [open, isEditMode, agentProfileId, agentProfiles, workspaceDefaults]);

  useEffect(() => {
    if (!open) return;
    const apiBaseUrl = getBackendConfig().apiBaseUrl;
    setEnvironmentsLoading(true);
    setExecutorsLoading(true);
    Promise.all([
      fetch(`${apiBaseUrl}/api/v1/environments`, { cache: 'no-store' })
        .then((response) => (response.ok ? response.json() : null))
        .then((data) => {
          if (!data?.environments) return [];
          return data.environments.map((env: { id: string; name: string; kind: string }) => ({
            id: env.id,
            name: env.name,
            kind: env.kind,
          }));
        })
        .catch(() => []),
      fetch(`${apiBaseUrl}/api/v1/executors`, { cache: 'no-store' })
        .then((response) => (response.ok ? response.json() : null))
        .then((data) => {
          if (!data?.executors) return [];
          return data.executors.map((executor: { id: string; name: string; type: string }) => ({
            id: executor.id,
            name: executor.name,
            type: executor.type,
          }));
        })
        .catch(() => []),
    ])
      .then(([nextEnvironments, nextExecutors]) => {
        setEnvironments(nextEnvironments);
        setExecutors(nextExecutors);
      })
      .finally(() => {
        setEnvironmentsLoading(false);
        setExecutorsLoading(false);
      });
  }, [open]);

  useEffect(() => {
    if (!open || isEditMode || environmentId || environments.length === 0) return;
    const defaultEnvironmentId = workspaceDefaults?.default_environment_id ?? null;
    if (
      defaultEnvironmentId &&
      environments.some((environment) => environment.id === defaultEnvironmentId)
    ) {
      setEnvironmentId(defaultEnvironmentId);
      return;
    }
    const localEnvironment = environments.find(
      (environment) => environment.kind === DEFAULT_LOCAL_ENVIRONMENT_KIND
    );
    setEnvironmentId(localEnvironment?.id ?? environments[0].id);
  }, [open, isEditMode, environmentId, environments, workspaceDefaults]);

  useEffect(() => {
    if (!open || isEditMode || executorId || executors.length === 0) return;
    const defaultExecutorId = workspaceDefaults?.default_executor_id ?? null;
    if (defaultExecutorId && executors.some((executor) => executor.id === defaultExecutorId)) {
      setExecutorId(defaultExecutorId);
      return;
    }
    const localExecutor = executors.find(
      (executor) => executor.type === DEFAULT_LOCAL_EXECUTOR_TYPE
    );
    setExecutorId(localExecutor?.id ?? executors[0].id);
  }, [open, isEditMode, executorId, executors, workspaceDefaults]);

  useEffect(() => {
    const textarea = descriptionRef.current;
    if (!textarea) return;
    textarea.style.height = 'auto';
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [description]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const trimmedTitle = title.trim();
    if (!trimmedTitle) return;
    if (isEditMode && editingTask) {
      try {
        const updatedTask = await updateTask(getBackendConfig().apiBaseUrl, editingTask.id, {
          title: trimmedTitle,
          description: description.trim(),
        });
        onSuccess?.(updatedTask, 'edit');
      } finally {
        setTitle('');
        setDescription('');
        setRepositoryId('');
        setBranch('');
        setStartAgent(false);
        setAgentProfileId('');
        onOpenChange(false);
      }
      return;
    }
    if (!workspaceId || !boardId) return;
    if (!repositoryId) return;
    const columnId = editingTask?.columnId ?? defaultColumnId;
    if (!columnId) return;
    let targetColumnId = columnId;
    let targetState: Task['state'] = 'CREATED';
    if (startAgent && !isEditMode) {
      const progressColumn = columns.find((column) =>
        column.title.toLowerCase().includes('progress')
      );
      targetColumnId = progressColumn?.id ?? columnId;
      targetState = 'IN_PROGRESS';
    }
    const repository = repositories.find((repo) => repo.id === repositoryId);
    try {
      const task = await createTask(getBackendConfig().apiBaseUrl, {
        workspace_id: workspaceId,
        board_id: boardId,
        column_id: targetColumnId,
        title: trimmedTitle,
        description: description.trim(),
        repository_url: repository?.local_path ?? '',
        branch: branch || '',
        agent_type: startAgent ? 'default' : '',
        environment_id: environmentId || undefined,
        executor_id: executorId || undefined,
        state: targetState,
      });
      onSuccess?.(task, 'create');
    } finally {
      setTitle('');
      setDescription('');
      setRepositoryId('');
      setBranch('');
      setStartAgent(false);
      setAgentProfileId('');
      setEnvironmentId('');
      setExecutorId('');
      onOpenChange(false);
    }
  };

  const handleCancel = () => {
    setTitle('');
    setDescription('');
    setRepositoryId('');
    setBranch('');
    setStartAgent(false);
    setAgentProfileId('');
    setEnvironmentId('');
    setExecutorId('');
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-none w-[900px] sm:!max-w-none max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>{submitLabel === 'Create' ? 'Create Task' : 'Edit Task'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4 overflow-hidden">
          <div className="flex-1 space-y-4 overflow-y-auto pr-1">
            <div>
              <Input
                autoFocus
                required
                placeholder="Enter task title..."
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
            </div>
            <div>
              <Textarea
                ref={descriptionRef}
                placeholder="Write a prompt for the agent..."
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                rows={2}
                className="min-h-[96px] max-h-[240px] resize-y overflow-auto"
              />
            </div>
            <div className="grid gap-4 md:grid-cols-3">
              <div>
                {repositories.length === 0 && !repositoriesLoading ? (
                  <div className="flex items-center justify-center h-10 px-3 py-2 text-sm border border-input rounded-md bg-background">
                    <span className="text-muted-foreground mr-2">No repositories found.</span>
                  <Link href="/settings/workspace" className="text-primary hover:underline">
                    Add repository
                  </Link>
                  </div>
                ) : (
                  <Combobox
                    options={repositories.map((repo) => ({
                      value: repo.id,
                      label: repo.name,
                      renderLabel: () => (
                        <span className="flex min-w-0 flex-1 items-center gap-2">
                          <span className="shrink-0">{repo.name}</span>
                          <Badge
                            variant="secondary"
                            className="text-xs text-muted-foreground max-w-[220px] min-w-0 truncate ml-auto"
                            title={formatUserHomePath(repo.local_path)}
                          >
                            {truncateRepoPath(repo.local_path)}
                          </Badge>
                        </span>
                      ),
                    }))}
                    value={repositoryId}
                    onValueChange={(value) => {
                      setRepositoryId(value);
                      setBranch('');
                    }}
                    placeholder={repositoriesLoading ? 'Loading repositories...' : 'Select repository'}
                    searchPlaceholder="Search repositories..."
                    emptyMessage="No repository found."
                    disabled={isEditMode || repositoriesLoading}
                    dropdownLabel="Repository"
                  />
                )}
              </div>
              <div>
                <Combobox
                  options={(branchesByRepo[repositoryId] ?? []).map((branchObj) => {
                    const displayName = branchObj.type === 'remote' && branchObj.remote
                      ? `${branchObj.remote}/${branchObj.name}`
                      : branchObj.name;
                    // Use display name as unique value since it includes remote prefix
                    return {
                      value: displayName,
                      label: displayName,
                      renderLabel: () => (
                        <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
                          <span className="truncate">{displayName}</span>
                          <Badge variant={branchObj.type === 'local' ? 'default' : 'secondary'} className="text-xs">
                            {branchObj.type}
                          </Badge>
                        </span>
                      ),
                    };
                  })}
                  value={branch}
                  onValueChange={(displayName) => {
                    // Store the full display name (e.g., "origin/master" or "master")
                    setBranch(displayName);
                  }}
                  placeholder={
                    !repositoryId
                      ? 'Select repository first'
                      : branchesLoading
                        ? 'Loading branches...'
                        : 'Select branch'
                  }
                  searchPlaceholder="Search branches..."
                  emptyMessage="No branch found."
                  disabled={isEditMode || !repositoryId || branchesLoading}
                  dropdownLabel="Branch"
                />
              </div>
              <div>
                {agentProfiles.length === 0 && !agentProfilesLoading ? (
                  <div className="flex items-center justify-center h-10 px-3 py-2 text-sm border border-input rounded-md bg-background">
                    <span className="text-muted-foreground mr-2">No agents found.</span>
                  <Link href="/settings/agents" className="text-primary hover:underline">
                    Add agent
                  </Link>
                  </div>
                ) : (
                  <Combobox
                    options={agentProfiles.map((profile) => ({
                      value: profile.id,
                      label: profile.label,
                    }))}
                    value={agentProfileId}
                    onValueChange={setAgentProfileId}
                    placeholder={
                      agentProfilesLoading
                        ? 'Loading agents...'
                        : agentProfiles.length === 0
                          ? 'No agents available'
                          : 'Select agent'
                    }
                    searchPlaceholder="Search agents..."
                    emptyMessage="No agent found."
                    dropdownLabel="Agent profile"
                  />
                )}
              </div>
            </div>
            <div className="flex items-center justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setShowAdvancedSettings(!showAdvancedSettings)}
                className="text-muted-foreground"
              >
                <IconSettings className="h-4 w-4 mr-1" />
                {showAdvancedSettings ? 'Hide' : 'More Options'}
              </Button>
            </div>
            {showAdvancedSettings && (
              <div className="grid gap-4 md:grid-cols-2">
                <div>
                  <Select value={environmentId} onValueChange={setEnvironmentId} disabled={isEditMode}>
                    <SelectTrigger className="w-full">
                      <SelectValue
                        placeholder={
                          environmentsLoading
                            ? 'Loading environments...'
                            : environments.length === 0
                              ? 'No environments available'
                              : 'Select environment'
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectLabel>Environment</SelectLabel>
                        {environments.map((environment) => (
                          <SelectItem key={environment.id} value={environment.id}>
                            {environment.name}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <Select value={executorId} onValueChange={setExecutorId} disabled={isEditMode}>
                    <SelectTrigger className="w-full">
                      <SelectValue
                        placeholder={
                          executorsLoading
                            ? 'Loading executors...'
                            : executors.length === 0
                              ? 'No executors available'
                              : 'Select executor'
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectLabel>Executor</SelectLabel>
                        {executors.map((executor) => (
                          <SelectItem key={executor.id} value={executor.id}>
                            {executor.name}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
              </div>
            )}
          </div>
          <DialogFooter className="border-t border-border pt-3">
            <div className="flex flex-1 items-center gap-3 text-sm text-muted-foreground">
              <div className="flex items-center gap-2">
                <input
                  id="start-agent"
                  type="checkbox"
                  checked={startAgent}
                  onChange={(e) => setStartAgent(e.target.checked)}
                  disabled={isEditMode}
                  className="h-4 w-4 rounded border border-input text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Label htmlFor="start-agent" className="cursor-pointer">
                        Start Agent
                      </Label>
                    </TooltipTrigger>
                    <TooltipContent>Start the agent on task creation.</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              </div>
              {startAgent && (
                <span className="text-xs text-muted-foreground">
                  A worktree will be created from this base branch.
                </span>
              )}
            </div>
            <DialogClose asChild>
              <Button type="button" variant="outline" onClick={handleCancel}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" disabled={!title.trim() || (!isEditMode && !repositoryId)}>
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
