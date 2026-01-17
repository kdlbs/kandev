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
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import { Combobox } from './combobox';
import { Badge } from '@kandev/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@kandev/ui/tooltip';
import type { LocalRepository, Task } from '@/lib/types/http';
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
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { useKeyboardShortcutHandler } from '@/hooks/use-keyboard-shortcut';
import { KeyboardShortcutTooltip } from '@/components/keyboard-shortcut-tooltip';
import { useToast } from '@/components/toast-provider';
import { discoverRepositoriesAction, listLocalRepositoryBranchesAction } from '@/app/actions/workspaces';

interface TaskCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode?: 'task' | 'session';
  workspaceId: string | null;
  boardId: string | null;
  defaultColumnId: string | null;
  columns: Array<{ id: string; title: string }>;
  editingTask?: { id: string; title: string; description?: string; columnId: string; state?: Task['state'] } | null;
  onSuccess?: (task: Task, mode: 'create' | 'edit', meta?: { taskSessionId?: string | null }) => void;
  onCreateSession?: (data: {
    prompt: string;
    agentProfileId: string;
    executorId: string;
    environmentId: string;
  }) => void;
  initialValues?: {
    title: string;
    description?: string;
    repositoryId?: string;
    branch?: string;
    state?: Task['state'];
  };
  submitLabel?: string;
}

type RepositoryOption = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
};

type RepositorySelectorProps = {
  options: RepositoryOption[];
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  searchPlaceholder: string;
  emptyMessage: string;
};

function RepositorySelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  searchPlaceholder,
  emptyMessage,
}: RepositorySelectorProps) {
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder={searchPlaceholder}
      emptyMessage={emptyMessage}
      disabled={disabled}
      dropdownLabel="Repository"
      className={disabled ? undefined : 'cursor-pointer'}
    />
  );
}

type BranchOption = {
  value: string;
  label: string;
  renderLabel: () => React.ReactNode;
};

type BranchSelectorProps = {
  options: BranchOption[];
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  searchPlaceholder: string;
  emptyMessage: string;
};

function BranchSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  searchPlaceholder,
  emptyMessage,
}: BranchSelectorProps) {
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder={searchPlaceholder}
      emptyMessage={emptyMessage}
      disabled={disabled}
      dropdownLabel="Base Branch"
      className={disabled ? undefined : 'cursor-pointer'}
    />
  );
}

type AgentSelectorProps = {
  options: Array<{ value: string; label: string; renderLabel: () => React.ReactNode }>;
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  triggerClassName?: string;
};

function AgentSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
}: AgentSelectorProps) {
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      searchPlaceholder="Search agents..."
      emptyMessage="No agent found."
      disabled={disabled}
      dropdownLabel="Agent profile"
      className={disabled ? undefined : 'cursor-pointer'}
      triggerClassName={triggerClassName}
    />
  );
}

export function TaskCreateDialog({
  open,
  onOpenChange,
  mode = 'task',
  workspaceId,
  boardId,
  defaultColumnId,
  columns,
  editingTask,
  onSuccess,
  onCreateSession,
  initialValues,
  submitLabel = 'Create',
}: TaskCreateDialogProps) {
  const isSessionMode = mode === 'session';
  const isEditMode = submitLabel !== 'Create';
  const [title, setTitle] = useState(initialValues?.title ?? '');
  const [description, setDescription] = useState(initialValues?.description ?? '');
  const [repositoryId, setRepositoryId] = useState(initialValues?.repositoryId ?? '');
  const [branch, setBranch] = useState(initialValues?.branch ?? '');
  const [startAgent, setStartAgent] = useState(!isEditMode);
  const [agentProfileId, setAgentProfileId] = useState('');
  const [environmentId, setEnvironmentId] = useState('');
  const [executorId, setExecutorId] = useState('');
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [discoveredRepoPath, setDiscoveredRepoPath] = useState('');
  const [selectedLocalRepo, setSelectedLocalRepo] = useState<LocalRepository | null>(null);
  const [localBranches, setLocalBranches] = useState<typeof branches>([]);
  const [localBranchesLoading, setLocalBranchesLoading] = useState(false);
  const [discoverReposLoading, setDiscoverReposLoading] = useState(false);
  const [discoverReposLoaded, setDiscoverReposLoaded] = useState(false);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const environments = useAppStore((state) => state.environments.items);
  const settingsData = useAppStore((state) => state.settingsData);
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null);
  const { toast } = useToast();
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
    setStartAgent(!isEditMode);
    setAgentProfileId('');
    setEnvironmentId('');
    setExecutorId('');
  }, [
    isEditMode,
    initialValues?.branch,
    initialValues?.description,
    initialValues?.repositoryId,
    initialValues?.title,
    open,
  ]);

  useEffect(() => {
    if (!open) return;
    setDiscoveredRepositories([]);
    setDiscoveredRepoPath('');
    setSelectedLocalRepo(null);
    setLocalBranches([]);
    setDiscoverReposLoaded(false);
  }, [open, workspaceId]);

  const workspaceDefaults = workspaceId
    ? workspaces.find((workspace) => workspace.id === workspaceId)
    : null;

  useEffect(() => {
    if (!open || !workspaceId) return;
    if (!repositoryId && !selectedLocalRepo && repositories.length === 1) {
      setRepositoryId(repositories[0].id);
    }
  }, [open, repositories, repositoryId, selectedLocalRepo, workspaceId]);

  useEffect(() => {
    if (!open || !workspaceId) return;
    if (repositoriesLoading) return;
    if (discoverReposLoaded || discoverReposLoading) return;
    setDiscoverReposLoading(true);
    discoverRepositoriesAction(workspaceId)
      .then((response) => {
        setDiscoveredRepositories(response.repositories);
      })
      .catch((error) => {
        toast({
          title: 'Failed to discover repositories',
          description: error instanceof Error ? error.message : 'Request failed',
          variant: 'error',
        });
        setDiscoveredRepositories([]);
      })
      .finally(() => {
        setDiscoverReposLoading(false);
        setDiscoverReposLoaded(true);
      });
  }, [
    discoverReposLoaded,
    discoverReposLoading,
    open,
    repositories.length,
    repositoriesLoading,
    toast,
    workspaceId,
  ]);

  useEffect(() => {
    if (!repositoryId) return;
    if (!branch) {
      const preferredBranch = selectPreferredBranch(branches);
      if (preferredBranch) {
        setBranch(preferredBranch);
      }
    }
  }, [branch, branches, repositoryId]);

  const handleSelectLocalRepository = (path: string) => {
    const selected = discoveredRepositories.find((repo) => repo.path === path) ?? null;
    setDiscoveredRepoPath(path);
    setSelectedLocalRepo(selected);
    setRepositoryId('');
    setBranch('');
    setLocalBranches([]);
  };

  const hasRepositorySelection = Boolean(repositoryId || selectedLocalRepo);
  const branchOptions = repositoryId
    ? branches
    : localBranches;
  const normalizeRepoPath = (path: string) => path.replace(/\\/g, '/').replace(/\/+$/g, '');
  const workspaceRepoPaths = new Set(
    repositories
      .map((repo) => repo.local_path)
      .filter(Boolean)
      .map((path) => normalizeRepoPath(path))
  );
  const localRepoOptions = discoveredRepositories.filter(
    (repo) => !workspaceRepoPaths.has(normalizeRepoPath(repo.path))
  );
  const repositoryOptions: RepositoryOption[] = [
    ...repositories.map((repo) => ({
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
    })),
    ...localRepoOptions.map((repo) => ({
      value: repo.path,
      label: formatUserHomePath(repo.path),
      renderLabel: () => (
        <span
          className="flex min-w-0 flex-1 items-center truncate"
          title={formatUserHomePath(repo.path)}
        >
          {truncateRepoPath(repo.path)}
        </span>
      ),
    })),
  ];

  useEffect(() => {
    if (!open || !workspaceId || !selectedLocalRepo) return;
    setLocalBranchesLoading(true);
    listLocalRepositoryBranchesAction(workspaceId, selectedLocalRepo.path)
      .then((response) => {
        setLocalBranches(response.branches);
      })
      .catch((error) => {
        toast({
          title: 'Failed to load branches',
          description: error instanceof Error ? error.message : 'Request failed',
          variant: 'error',
        });
        setLocalBranches([]);
      })
      .finally(() => {
        setLocalBranchesLoading(false);
      });
  }, [open, selectedLocalRepo, toast, workspaceId]);

  useEffect(() => {
    if (repositoryId || localBranches.length === 0) return;
    if (branch) return;
    const preferredBranch = selectPreferredBranch(localBranches);
    if (preferredBranch) {
      setBranch(preferredBranch);
    }
  }, [branch, localBranches, repositoryId]);

  useEffect(() => {
    if (!open || agentProfileId || agentProfiles.length === 0) return;
    if (!isSessionMode && !startAgent) return;
    if (isEditMode && agentProfiles.length > 1) return;
    const defaultProfileId = workspaceDefaults?.default_agent_profile_id ?? null;
    if (defaultProfileId && agentProfiles.some((profile) => profile.id === defaultProfileId)) {
      setAgentProfileId(defaultProfileId);
      return;
    }
    setAgentProfileId(agentProfiles[0].id);
  }, [open, isEditMode, agentProfileId, agentProfiles, workspaceDefaults, isSessionMode, startAgent]);

  useEffect(() => {
    if (!open || isEditMode || environmentId || environments.length === 0) return;
    if (!isSessionMode && !startAgent) return;
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
  }, [open, isEditMode, environmentId, environments, workspaceDefaults, isSessionMode, startAgent]);

  useEffect(() => {
    if (!open || isEditMode || executorId || executors.length === 0) return;
    if (!isSessionMode && !startAgent) return;
    const defaultExecutorId = workspaceDefaults?.default_executor_id ?? null;
    if (defaultExecutorId && executors.some((executor) => executor.id === defaultExecutorId)) {
      setExecutorId(defaultExecutorId);
      return;
    }
    const localExecutor = executors.find(
      (executor) => executor.type === DEFAULT_LOCAL_EXECUTOR_TYPE
    );
    setExecutorId(localExecutor?.id ?? executors[0].id);
  }, [open, isEditMode, executorId, executors, workspaceDefaults, isSessionMode, startAgent]);

  useEffect(() => {
    const textarea = descriptionRef.current;
    if (!textarea) return;
    textarea.style.height = 'auto';
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [description]);

  // Use keyboard shortcut hook for Cmd+Enter / Ctrl+Enter
  const handleKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, (event) => {
    handleSubmit(event as unknown as FormEvent);
  });

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();

    // Session mode - create a new session
    if (isSessionMode) {
      const trimmedDescription = description.trim();
      if (!trimmedDescription || !agentProfileId) return;

      onCreateSession?.({
        prompt: trimmedDescription,
        agentProfileId,
        executorId,
        environmentId,
      });

      // Reset form
      setDescription('');
      setAgentProfileId('');
      setExecutorId('');
      setEnvironmentId('');
      setShowAdvancedSettings(false);
      onOpenChange(false);
      return;
    }

    // Task mode - create or edit task
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
    if (!repositoryId && !selectedLocalRepo) return;
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
    try {
      const taskResponse = await createTask({
        workspace_id: workspaceId,
        board_id: boardId,
        column_id: targetColumnId,
        title: trimmedTitle,
        description: description.trim(),
        repositories: repositoryId
          ? [
            {
              repository_id: repositoryId,
              base_branch: branch || undefined,
            },
          ]
          : selectedLocalRepo
            ? [
              {
                repository_id: '',
                base_branch: branch || undefined,
                local_path: selectedLocalRepo.path,
                default_branch: selectedLocalRepo.default_branch || undefined,
              },
            ]
            : [],
        state: targetState,
        start_agent: Boolean(startAgent && !isEditMode && agentProfileId),
        agent_profile_id: startAgent ? agentProfileId || undefined : undefined,
      });
      console.log('[TaskCreateDialog] task created', {
        taskId: taskResponse.id,
        startAgent,
        agentProfileId,
      });
      onSuccess?.(taskResponse, 'create', {
        taskSessionId: taskResponse.session_id ?? null,
      });
    } finally {
      setTitle('');
      setDescription('');
      setRepositoryId('');
      setBranch('');
      setStartAgent(!isEditMode);
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
    setStartAgent(!isEditMode);
    setAgentProfileId('');
    setEnvironmentId('');
    setExecutorId('');
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-none w-[900px] sm:!max-w-none max-h-[85vh] flex flex-col bg-card">
        <DialogHeader>
          <DialogTitle>
            {isSessionMode ? 'Create New Session' : submitLabel === 'Create' ? 'Create Task' : 'Edit Task'}
          </DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4 overflow-hidden">
          <div className="flex-1 space-y-4 overflow-y-auto pr-1">
            {!isSessionMode && (
              <div>
                <Input
                  autoFocus
                  required
                  placeholder="Enter task title..."
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  disabled={isSessionMode}
                />
              </div>
            )}
            {isSessionMode && (
              <div>
                <Label htmlFor="task-title">Task</Label>
                <Input
                  id="task-title"
                  value={title}
                  disabled
                  placeholder="Task"
                  className="bg-muted cursor-not-allowed mt-1.5"
                />
              </div>
            )}
            <div>
              {isSessionMode && <Label htmlFor="prompt">Prompt</Label>}
              <Textarea
                id={isSessionMode ? 'prompt' : undefined}
                ref={descriptionRef}
                placeholder={isSessionMode ? 'Describe what you want the agent to do...' : 'Write a prompt for the agent...'}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                onKeyDown={handleKeyDown}
                rows={2}
                className={isSessionMode ? 'min-h-[120px] resize-none mt-1.5' : 'min-h-[96px] max-h-[240px] resize-y overflow-auto'}
                required={isSessionMode}
              />
            </div>
            {!isSessionMode && (
              <div className="grid gap-4 md:grid-cols-3">
                <div>
                  <RepositorySelector
                    options={repositoryOptions}
                    value={repositoryId || discoveredRepoPath}
                    onValueChange={(value) => {
                      const workspaceRepo = repositories.find((repo) => repo.id === value);
                      if (workspaceRepo) {
                        setRepositoryId(value);
                        setDiscoveredRepoPath('');
                        setSelectedLocalRepo(null);
                        setLocalBranches([]);
                        setBranch('');
                        return;
                      }
                      handleSelectLocalRepository(value);
                    }}
                    placeholder={
                      !workspaceId
                        ? 'Select workspace first'
                        : repositoriesLoading || discoverReposLoading
                          ? 'Loading repositories...'
                          : 'Select repository'
                    }
                    searchPlaceholder="Search repositories..."
                    emptyMessage={
                      repositoriesLoading || discoverReposLoading
                        ? 'Loading repositories...'
                        : 'No repositories found.'
                    }
                    disabled={!workspaceId || repositoriesLoading || discoverReposLoading || isEditMode}
                  />
                </div>
              <div>
                <BranchSelector
                  options={branchOptions.map((branchObj) => {
                    const displayName =
                      branchObj.type === 'remote' && branchObj.remote
                        ? `${branchObj.remote}/${branchObj.name}`
                        : branchObj.name;
                    // Use display name as unique value since it includes remote prefix
                    return {
                      value: displayName,
                      label: displayName,
                      renderLabel: () => (
                        <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
                          <span className="truncate" title={displayName}>
                            {displayName}
                          </span>
                          <Badge variant={branchObj.type === 'local' ? 'default' : 'secondary'} className="text-xs">
                            {branchObj.type === 'local' ? 'local' : branchObj.remote || 'remote'}
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
                    !hasRepositorySelection
                      ? 'Select repository first'
                      : repositoryId
                        ? branchesLoading
                          ? 'Loading branches...'
                          : 'Select branch'
                        : localBranchesLoading
                          ? 'Loading branches...'
                          : branchOptions.length > 0
                            ? 'Select branch'
                            : 'No branches found'
                  }
                  searchPlaceholder="Search branches..."
                  emptyMessage="No branch found."
                  disabled={
                    isEditMode ||
                    !hasRepositorySelection ||
                    (repositoryId && branchesLoading) ||
                    (!repositoryId && (localBranchesLoading || branchOptions.length === 0))
                  }
                />
              </div>
                {startAgent && (
                  <div>
                    {agentProfiles.length === 0 && !agentProfilesLoading ? (
                      <div className="flex h-7 items-center justify-center gap-2 rounded-sm border border-input px-3 text-xs text-muted-foreground">
                        <span>No agents found.</span>
                        <Link href="/settings/agents" className="text-primary hover:underline">
                          Add agent
                        </Link>
                      </div>
                    ) : (
                    <AgentSelector
                      options={agentProfiles.map((profile) => ({
                        value: profile.id,
                        label: profile.label,
                        renderLabel: () => {
                          const parts = profile.label.split(' • ');
                          const agentLabel = parts[0] ?? profile.label;
                          const profileLabel = parts[1] ?? '';
                          return (
                            <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
                              <span className="truncate">{agentLabel}</span>
                              {profileLabel ? (
                                <Badge variant="secondary" className="text-xs">
                                  {profileLabel}
                                </Badge>
                              ) : null}
                            </span>
                          );
                        },
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
                      disabled={agentProfilesLoading}
                    />
                    )}
                  </div>
                )}
              </div>
            )}

            {/* Agent Profile - shown in session mode */}
            {isSessionMode && (
              <div className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-3">
                  <AgentSelector
                    options={agentProfiles.map((profile) => ({
                      value: profile.id,
                      label: profile.label,
                      renderLabel: () => {
                        const parts = profile.label.split(' • ');
                        const agentLabel = parts[0] ?? profile.label;
                        const profileLabel = parts[1] ?? '';
                        return (
                          <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
                            <span className="truncate">{agentLabel}</span>
                            {profileLabel ? (
                              <Badge variant="secondary" className="text-xs">
                                {profileLabel}
                              </Badge>
                            ) : null}
                          </span>
                        );
                      },
                    }))}
                    value={agentProfileId}
                    onValueChange={setAgentProfileId}
                    placeholder={agentProfilesLoading ? 'Loading agent profiles...' : 'Select agent profile'}
                    disabled={agentProfilesLoading}
                    triggerClassName="w-[280px]"
                  />
                </div>

                {/* More Options Toggle */}
                <button
                  type="button"
                  onClick={() => setShowAdvancedSettings(!showAdvancedSettings)}
                  className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors cursor-pointer whitespace-nowrap"
                >
                  <IconSettings className="h-4 w-4" />
                  More options
                </button>
              </div>
            )}

            {!isSessionMode && startAgent && (
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
            )}
            {showAdvancedSettings && isSessionMode && (
              <div className="pt-2 border-t">
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="flex items-center gap-3">
                    <Label htmlFor="environment" className="text-sm whitespace-nowrap">
                      Environment
                    </Label>
                    <Select value={environmentId} onValueChange={setEnvironmentId}>
                      <SelectTrigger id="environment" className="w-full">
                        <SelectValue placeholder={environmentsLoading ? 'Loading environments...' : 'Select environment'} />
                      </SelectTrigger>
                      <SelectContent>
                        {environments.map((env) => (
                          <SelectItem key={env.id} value={env.id}>
                            {env.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="flex items-center gap-3">
                    <Label htmlFor="executor" className="text-sm whitespace-nowrap">
                      Executor
                    </Label>
                    <Select value={executorId} onValueChange={setExecutorId}>
                      <SelectTrigger id="executor" className="w-full">
                        <SelectValue placeholder={executorsLoading ? 'Loading executors...' : 'Select executor'} />
                      </SelectTrigger>
                      <SelectContent>
                        {executors.map((executor) => (
                          <SelectItem key={executor.id} value={executor.id}>
                            {executor.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>
            )}
            {showAdvancedSettings && !isSessionMode && startAgent && (
              <div className="grid gap-4 md:grid-cols-2">
                <div className="flex items-center gap-3">
                  <Label className="text-sm whitespace-nowrap">Environment</Label>
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
                      {environments.map((environment) => (
                        <SelectItem key={environment.id} value={environment.id}>
                          {environment.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex items-center gap-3">
                  <Label className="text-sm whitespace-nowrap">Executor</Label>
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
                      {executors.map((executor) => (
                        <SelectItem key={executor.id} value={executor.id}>
                          {executor.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            )}
          </div>
          <DialogFooter className="border-t border-border pt-3">
            {!isSessionMode && (
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
                    A git worktree will be created from the base branch.
                  </span>
                )}
              </div>
            )}
            <DialogClose asChild>
              <Button type="button" variant="outline" onClick={handleCancel}>
                Cancel
              </Button>
            </DialogClose>
            <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT}>
              <Button
                type="submit"
                disabled={
                  isSessionMode
                    ? !description.trim() || !agentProfileId
                    : !title.trim() ||
                    !description.trim() ||
                    (!isEditMode && !repositoryId && !selectedLocalRepo) ||
                    (startAgent && !agentProfileId)
                }
              >
                {isSessionMode ? 'Create Session' : submitLabel}
              </Button>
            </KeyboardShortcutTooltip>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
