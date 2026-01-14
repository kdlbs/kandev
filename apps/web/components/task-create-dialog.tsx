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
import type { Task } from '@/lib/types/http';
import {
  DEFAULT_LOCAL_ENVIRONMENT_KIND,
  DEFAULT_LOCAL_EXECUTOR_TYPE,
  formatUserHomePath,
  selectPreferredBranch,
  truncateRepoPath,
} from '@/lib/utils';
import { createTask, updateTask } from '@/lib/http';
import { useAppStore } from '@/components/state-provider';
import { useRepositories } from '@/hooks/use-repositories';
import { useRepositoryBranches } from '@/hooks/use-repository-branches';
import { useSettingsData } from '@/hooks/use-settings-data';

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
  const [agentProfileId, setAgentProfileId] = useState('');
  const [environmentId, setEnvironmentId] = useState('');
  const [executorId, setExecutorId] = useState('');
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const environments = useAppStore((state) => state.environments.items);
  const settingsData = useAppStore((state) => state.settingsData);
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null);
  useSettingsData(open);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, open);
  const { branches, isLoading: branchesLoading } = useRepositoryBranches(
    repositoryId || null,
    Boolean(open && repositoryId)
  );
  const agentProfilesLoading = open && !settingsData.agentsLoaded;
  const environmentsLoading = open && !settingsData.environmentsLoaded;
  const executorsLoading = open && !settingsData.executorsLoaded;

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
    if (!repositoryId && repositories.length === 1) {
      setRepositoryId(repositories[0].id);
    }
  }, [open, repositories, repositoryId, workspaceId]);

  useEffect(() => {
    if (!repositoryId) return;
    if (!branch) {
      const preferredBranch = selectPreferredBranch(branches);
      if (preferredBranch) {
        setBranch(preferredBranch);
      }
    }
  }, [branch, branches, repositoryId]);

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
        const updatedTask = await updateTask(editingTask.id, {
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
      const task = await createTask({
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
      <DialogContent className="max-w-none w-[900px] sm:!max-w-none max-h-[85vh] flex flex-col bg-card">
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
                    className={isEditMode || repositoriesLoading ? undefined : 'cursor-pointer'}
                  />
                )}
              </div>
              <div>
                <Combobox
                  options={branches.map((branchObj) => {
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
                  dropdownLabel="Base Branch"
                  className={isEditMode || !repositoryId || branchesLoading ? undefined : 'cursor-pointer'}
                />
              </div>
              <div>
                {agentProfiles.length === 0 && !agentProfilesLoading ? (
                  <div className="flex h-7 items-center justify-center gap-2 rounded-sm border border-input px-3 text-xs text-muted-foreground">
                    <span>No agents found.</span>
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
                    className={agentProfilesLoading ? undefined : 'cursor-pointer'}
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
                className="text-muted-foreground cursor-pointer"
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
                  A git worktree will be created from the above branch.
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
