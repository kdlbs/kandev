'use client';

import { useEffect, useRef, useState, FormEvent, memo, useMemo, useCallback } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { IconLoader2, IconGitBranch, IconFileInvoice, IconSend, IconChevronDown } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from '@kandev/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Textarea } from '@kandev/ui/textarea';
import { Button } from '@kandev/ui/button';
import { Combobox } from './combobox';
import { Badge } from '@kandev/ui/badge';
import { ScrollOnOverflow } from '@kandev/ui/scroll-on-overflow';
import type { LocalRepository, Task, Workspace, Repository, Environment, Executor, Branch } from '@/lib/types/http';
import type { AgentProfileOption } from '@/lib/state/slices';
import {
  DEFAULT_LOCAL_ENVIRONMENT_KIND,
  DEFAULT_LOCAL_EXECUTOR_TYPE,
  formatUserHomePath,
  selectPreferredBranch,
  truncateRepoPath,
} from '@/lib/utils';
import { createTask, updateTask } from '@/lib/api';
import { useAppStore } from '@/components/state-provider';
import { useRepositories } from '@/hooks/domains/workspace/use-repositories';
import { useRepositoryBranches } from '@/hooks/domains/workspace/use-repository-branches';
import { useSettingsData } from '@/hooks/domains/settings/use-settings-data';
import { getWebSocketClient } from '@/lib/ws/connection';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { useKeyboardShortcutHandler } from '@/hooks/use-keyboard-shortcut';
import { KeyboardShortcutTooltip } from '@/components/keyboard-shortcut-tooltip';
import { useToast } from '@/components/toast-provider';
import { discoverRepositoriesAction, listLocalRepositoryBranchesAction } from '@/app/actions/workspaces';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { STORAGE_KEYS } from '@/lib/settings/constants';
import { linkToSession } from '@/lib/links';
import { getExecutorIcon } from '@/lib/executor-icons';
import { AgentLogo } from '@/components/agent-logo';

import { useDockviewStore } from '@/lib/state/dockview-store';

interface TaskCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode?: 'create' | 'edit' | 'session';
  workspaceId: string | null;
  boardId: string | null;
  defaultColumnId: string | null;
  columns: Array<{ id: string; title: string; autoStartAgent?: boolean }>;
  editingTask?: { id: string; title: string; description?: string; workflowStepId: string; state?: Task['state']; repositoryId?: string } | null;
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
  taskId?: string | null;
}

interface OrchestratorStartResponse {
  success: boolean;
  task_id: string;
  agent_instance_id: string;
  session_id?: string;
  state: string;
}

function autoSelectBranch(branchList: Branch[], setBranch: (value: string) => void): void {
  const lastUsedBranch = getLocalStorage<string | null>(STORAGE_KEYS.LAST_BRANCH, null);
  if (lastUsedBranch && branchList.some((b) => {
    const displayName = b.type === 'remote' && b.remote ? `${b.remote}/${b.name}` : b.name;
    return displayName === lastUsedBranch;
  })) {
    setBranch(lastUsedBranch);
    return;
  }
  const preferredBranch = selectPreferredBranch(branchList);
  if (preferredBranch) {
    setBranch(preferredBranch);
  }
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
  triggerClassName?: string;
};

const RepositorySelector = memo(function RepositorySelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  searchPlaceholder,
  emptyMessage,
  triggerClassName,
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
      triggerClassName={triggerClassName}
    />
  );
});

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

const BranchSelector = memo(function BranchSelector({
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
});

type AgentSelectorProps = {
  options: Array<{ value: string; label: string; renderLabel: () => React.ReactNode }>;
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  triggerClassName?: string;
};

const AgentSelector = memo(function AgentSelector({
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
      className={`min-w-[380px]${disabled ? '' : ' cursor-pointer'}`}
      triggerClassName={triggerClassName}
    />
  );
});

type ExecutorSelectorProps = {
  options: Array<{ value: string; label: string; renderLabel?: () => React.ReactNode }>;
  value: string;
  onValueChange: (value: string) => void;
  disabled: boolean;
  placeholder: string;
  triggerClassName?: string;
};

const ExecutorSelector = memo(function ExecutorSelector({
  options,
  value,
  onValueChange,
  disabled,
  placeholder,
  triggerClassName,
}: ExecutorSelectorProps) {
  return (
    <Combobox
      options={options}
      value={value}
      onValueChange={onValueChange}
      placeholder={placeholder}
      emptyMessage="No executor found."
      disabled={disabled}
      dropdownLabel="Executor"
      className={disabled ? undefined : 'cursor-pointer'}
      triggerClassName={triggerClassName}
      showSearch={false}
    />
  );
});

type InlineTaskNameProps = {
  value: string;
  onChange: (value: string) => void;
  autoFocus?: boolean;
};

const InlineTaskName = memo(function InlineTaskName({
  value,
  onChange,
  autoFocus,
}: InlineTaskNameProps) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (autoFocus && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [autoFocus]);

  return (
    <input
      ref={inputRef}
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder="task-name"
      size={Math.max(value.length, 9)}
      className="bg-transparent border-none outline-none focus:ring-0 text-sm font-medium min-w-0 rounded-md px-1.5 py-0.5 -mx-1.5 hover:bg-muted focus:bg-muted transition-colors"
    />
  );
});

// Memoized description input to prevent re-rendering the entire dialog on every keystroke
type TaskFormInputsProps = {
  isSessionMode: boolean;
  autoFocus?: boolean;
  initialDescription: string;
  onDescriptionChange: (hasContent: boolean) => void;
  onKeyDown: (e: React.KeyboardEvent) => void;
  descriptionValueRef: React.RefObject<{ getValue: () => string } | null>;
};

const TaskFormInputs = memo(function TaskFormInputs({
  isSessionMode,
  autoFocus,
  initialDescription,
  onDescriptionChange,
  onKeyDown,
  descriptionValueRef,
}: TaskFormInputsProps) {
  const [description, setDescription] = useState(initialDescription);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    const ref = descriptionValueRef as React.MutableRefObject<{ getValue: () => string } | null>;
    if (ref) {
      ref.current = { getValue: () => description };
    }
  }, [description, descriptionValueRef]);

  // Auto-resize textarea + optional auto-focus with cursor at end
  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.style.height = 'auto';
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [description]);

  useEffect(() => {
    if (!autoFocus) return;
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.focus();
    textarea.setSelectionRange(textarea.value.length, textarea.value.length);
  }, [autoFocus]);

  const handleDescriptionChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newValue = e.target.value;
    const hadContent = description.trim().length > 0;
    const hasContent = newValue.trim().length > 0;
    setDescription(newValue);
    if (hadContent !== hasContent) {
      onDescriptionChange(hasContent);
    }
  }, [description, onDescriptionChange]);

  return (
    <div>
      <Textarea
        ref={textareaRef}
        placeholder={isSessionMode ? 'Describe what you want the agent to do...' : 'Write a prompt for the agent...'}
        value={description}
        onChange={handleDescriptionChange}
        onKeyDown={onKeyDown}
        rows={2}
        className={isSessionMode ? 'min-h-[120px] max-h-[240px] resize-none overflow-auto' : 'min-h-[96px] max-h-[240px] resize-y overflow-auto'}
        required={isSessionMode}
      />
    </div>
  );
});

export function TaskCreateDialog({
  open,
  onOpenChange,
  mode = 'create',
  workspaceId,
  boardId,
  defaultColumnId,
  columns,
  editingTask,
  onSuccess,
  onCreateSession,
  initialValues,
  taskId = null,
}: TaskCreateDialogProps) {
  const router = useRouter();
  const isSessionMode = mode === 'session';
  const isEditMode = mode === 'edit';
  const isCreateMode = mode === 'create';
  const [isCreatingSession, setIsCreatingSession] = useState(false);
  const [isCreatingTask, setIsCreatingTask] = useState(false);
  const [taskName, setTaskName] = useState('');
  // Track only whether inputs have content (not the actual values) to minimize re-renders
  const [hasTitle, setHasTitle] = useState(Boolean(initialValues?.title?.trim()));
  const [hasDescription, setHasDescription] = useState(Boolean(initialValues?.description?.trim()));
  const descriptionInputRef = useRef<{ getValue: () => string } | null>(null);
  const [repositoryId, setRepositoryId] = useState(initialValues?.repositoryId ?? '');
  const [branch, setBranch] = useState(initialValues?.branch ?? '');
  const [agentProfileId, setAgentProfileId] = useState('');
  const [environmentId, setEnvironmentId] = useState('');
  const [executorId, setExecutorId] = useState('');
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
  // Derive repository name from store for session mode header
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);
  const reposByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const sessionRepoName = useMemo(() => {
    if (!isSessionMode) return undefined;
    const activeTask = activeTaskId ? kanbanTasks.find((t) => t.id === activeTaskId) : null;
    const repoId = activeTask?.repositoryId;
    if (!repoId) return undefined;
    for (const repos of Object.values(reposByWorkspace)) {
      const repo = repos.find((r) => r.id === repoId);
      if (repo) return repo.name;
    }
    return undefined;
  }, [isSessionMode, activeTaskId, kanbanTasks, reposByWorkspace]);
  const { toast } = useToast();
  useSettingsData(open);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, open);
  const { branches, isLoading: branchesLoading } = useRepositoryBranches(
    repositoryId || null,
    Boolean(open && repositoryId)
  );
  const agentProfilesLoading = open && !settingsData.agentsLoaded;
  const executorsLoading = open && !settingsData.executorsLoaded;

  useEffect(() => {
    if (!open) return;
    const name = initialValues?.title || '';
    setTaskName(name);
    setHasTitle(name.trim().length > 0);
    setHasDescription(Boolean(initialValues?.description?.trim()));
    setRepositoryId(initialValues?.repositoryId ?? '');
    setBranch(initialValues?.branch ?? '');
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

  useEffect(() => {
    if (!open) return;
    setDiscoveredRepositories([]);
    setDiscoveredRepoPath('');
    setSelectedLocalRepo(null);
    setLocalBranches([]);
    setDiscoverReposLoaded(false);
  }, [open, workspaceId]);

  const workspaceDefaults = workspaceId
    ? workspaces.find((workspace: Workspace) => workspace.id === workspaceId)
    : null;

  useEffect(() => {
    if (!open || !workspaceId) return;
    if (repositoryId || selectedLocalRepo) return;

    // Priority 1: Last used repository
    const lastUsedRepoId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null);
    if (lastUsedRepoId && repositories.some((repo: Repository) => repo.id === lastUsedRepoId)) {
      setRepositoryId(lastUsedRepoId);
      return;
    }

    // Priority 2: Auto-select if only one repository
    if (repositories.length === 1) {
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
    if (!repositoryId || branch) return;
    autoSelectBranch(branches, setBranch);
  }, [branch, branches, repositoryId]);

  const handleSelectLocalRepository = useCallback((path: string) => {
    const selected = discoveredRepositories.find((repo) => repo.path === path) ?? null;
    setDiscoveredRepoPath(path);
    setSelectedLocalRepo(selected);
    setRepositoryId('');
    setBranch('');
    setLocalBranches([]);
  }, [discoveredRepositories]);

  // Memoized callback for RepositorySelector to prevent re-renders when typing
  const handleRepositoryChange = useCallback((value: string) => {
    const workspaceRepo = repositories.find((repo: Repository) => repo.id === value);
    if (workspaceRepo) {
      setRepositoryId(value);
      setLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID, value);
      setDiscoveredRepoPath('');
      setSelectedLocalRepo(null);
      setLocalBranches([]);
      setBranch('');
      return;
    }
    handleSelectLocalRepository(value);
  }, [repositories, handleSelectLocalRepository]);

  // Memoized callback for AgentSelector to save selection to localStorage
  const handleAgentProfileChange = useCallback((value: string) => {
    setAgentProfileId(value);
    setLocalStorage(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, value);
  }, []);

  // Memoized callback for InlineTaskName
  const handleTaskNameChange = useCallback((value: string) => {
    setTaskName(value);
    setHasTitle(value.trim().length > 0);
  }, []);

  // Memoized callback for BranchSelector to save selection to localStorage
  const handleBranchChange = useCallback((value: string) => {
    setBranch(value);
    setLocalStorage(STORAGE_KEYS.LAST_BRANCH, value);
  }, []);

  const hasRepositorySelection = Boolean(repositoryId || selectedLocalRepo);
  const branchOptionsRaw = repositoryId
    ? branches
    : localBranches;

  // Memoize repository options to prevent re-renders when typing
  const repositoryOptions = useMemo(() => {
    const normalizeRepoPath = (path: string) => path.replace(/\\/g, '/').replace(/\/+$/g, '');
    const workspaceRepoPaths = new Set(
      repositories
        .map((repo: Repository) => repo.local_path)
        .filter(Boolean)
        .map((path: string) => normalizeRepoPath(path))
    );
    const localRepoOptions = discoveredRepositories.filter(
      (repo: LocalRepository) => !workspaceRepoPaths.has(normalizeRepoPath(repo.path))
    );
    return [
      ...repositories.map((repo: Repository) => ({
        value: repo.id,
        label: repo.name,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
            <span className="shrink-0">{repo.name}</span>
            <Badge
              variant="secondary"
              className="text-xs text-muted-foreground max-w-[140px] min-w-0 truncate ml-auto"
              title={formatUserHomePath(repo.local_path)}
            >
              {truncateRepoPath(repo.local_path, 24)}
            </Badge>
          </span>
        ),
      })),
      ...localRepoOptions.map((repo: LocalRepository) => ({
        value: repo.path,
        label: truncateRepoPath(repo.path, 24),
        renderLabel: () => (
          <span
            className="flex min-w-0 flex-1 items-center overflow-hidden"
            title={formatUserHomePath(repo.path)}
          >
            <span className="truncate">{truncateRepoPath(repo.path, 28)}</span>
          </span>
        ),
      })),
    ];
  }, [repositories, discoveredRepositories]);

  // Header-only repository options: name only, no path badge
  const headerRepositoryOptions = useMemo(() => {
    return repositoryOptions.map((opt) => ({
      ...opt,
      renderLabel: () => (
        <span className="truncate">{opt.label}</span>
      ),
    }));
  }, [repositoryOptions]);

  // Memoize branch options to prevent re-renders when typing
  const branchOptions = useMemo(() => {
    return branchOptionsRaw.map((branchObj: Branch) => {
      const displayName =
        branchObj.type === 'remote' && branchObj.remote
          ? `${branchObj.remote}/${branchObj.name}`
          : branchObj.name;
      return {
        value: displayName,
        label: displayName,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
            <span className="flex min-w-0 items-center gap-1.5">
              <IconGitBranch className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <span className="truncate" title={displayName}>
                {displayName}
              </span>
            </span>
            <Badge variant="outline" className="text-xs">
              {branchObj.type === 'local' ? 'local' : branchObj.remote || 'remote'}
            </Badge>
          </span>
        ),
      };
    });
  }, [branchOptionsRaw]);

  // Memoize agent profile options to prevent re-renders when typing
  const agentProfileOptions = useMemo(() => {
    return agentProfiles.map((profile: AgentProfileOption) => {
      const parts = profile.label.split(' • ');
      const agentLabel = parts[0] ?? profile.label;
      const profileLabel = parts[1] ?? '';
      return {
        value: profile.id,
        label: profile.label,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center justify-between gap-2">
            <span className="flex shrink-0 items-center gap-1.5">
              <AgentLogo agentName={profile.agent_name} className="shrink-0" />
              <span>{agentLabel}</span>
            </span>
            {profileLabel ? (
              <ScrollOnOverflow className="rounded-full border border-border px-2 py-0.5 text-xs">
                {profileLabel}
              </ScrollOnOverflow>
            ) : null}
          </span>
        ),
      };
    });
  }, [agentProfiles]);

  const executorOptions = useMemo(() => {
    return executors.map((executor: Executor) => {
      const Icon = getExecutorIcon(executor.type);
      return {
        value: executor.id,
        label: executor.name,
        renderLabel: () => (
          <span className="flex min-w-0 flex-1 items-center gap-2">
            <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            <span className="truncate">{executor.name}</span>
          </span>
        ),
      };
    });
  }, [executors]);

  const executorHint = useMemo(() => {
    const selectedExecutor = executors.find((e: Executor) => e.id === executorId);
    if (selectedExecutor?.type === 'worktree') return 'A git worktree will be created from the base branch.';
    if (selectedExecutor?.type === 'local') return 'The agent will run directly on the repository.';
    return null;
  }, [executors, executorId]);

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
    if (repositoryId || localBranches.length === 0 || branch) return;
    autoSelectBranch(localBranches, setBranch);
  }, [branch, localBranches, repositoryId]);

  useEffect(() => {
    if (!open || agentProfileId || agentProfiles.length === 0) return;

    // Priority: last used → workspace default → first available
    const lastUsedProfileId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, null);
    if (lastUsedProfileId && agentProfiles.some((profile: AgentProfileOption) => profile.id === lastUsedProfileId)) {
      setAgentProfileId(lastUsedProfileId);
      return;
    }

    const defaultProfileId = workspaceDefaults?.default_agent_profile_id ?? null;
    if (defaultProfileId && agentProfiles.some((profile: AgentProfileOption) => profile.id === defaultProfileId)) {
      setAgentProfileId(defaultProfileId);
      return;
    }
    setAgentProfileId(agentProfiles[0].id);
  }, [open, agentProfileId, agentProfiles, workspaceDefaults]);

  useEffect(() => {
    if (!open || environmentId || environments.length === 0) return;
    const defaultEnvironmentId = workspaceDefaults?.default_environment_id ?? null;
    if (
      defaultEnvironmentId &&
      environments.some((environment: Environment) => environment.id === defaultEnvironmentId)
    ) {
      setEnvironmentId(defaultEnvironmentId);
      return;
    }
    const localEnvironment = environments.find(
      (environment: Environment) => environment.kind === DEFAULT_LOCAL_ENVIRONMENT_KIND
    );
    setEnvironmentId(localEnvironment?.id ?? environments[0].id);
  }, [open, environmentId, environments, workspaceDefaults]);

  useEffect(() => {
    if (!open || executorId || executors.length === 0) return;
    const defaultExecutorId = workspaceDefaults?.default_executor_id ?? null;
    if (defaultExecutorId && executors.some((executor: Executor) => executor.id === defaultExecutorId)) {
      setExecutorId(defaultExecutorId);
      return;
    }
    const localExecutor = executors.find(
      (executor: Executor) => executor.type === DEFAULT_LOCAL_EXECUTOR_TYPE
    );
    setExecutorId(localExecutor?.id ?? executors[0].id);
  }, [open, executorId, executors, workspaceDefaults]);

  // Access layout/UI store for plan mode creation
  const setActiveDocument = useAppStore((state) => state.setActiveDocument);
  const setPlanMode = useAppStore((state) => state.setPlanMode);

  const resetForm = useCallback(() => {
    setHasTitle(false);
    setHasDescription(false);
    setTaskName('');
    setRepositoryId('');
    setBranch('');
    setAgentProfileId('');
    setEnvironmentId('');
    setExecutorId('');
  }, []);

  const getRepositoriesPayload = useCallback(() => {
    if (repositoryId) {
      return [{ repository_id: repositoryId, base_branch: branch || undefined }];
    }
    if (selectedLocalRepo) {
      return [{
        repository_id: '',
        base_branch: branch || undefined,
        local_path: selectedLocalRepo.path,
        default_branch: selectedLocalRepo.default_branch || undefined,
      }];
    }
    return [];
  }, [repositoryId, branch, selectedLocalRepo]);

  // --- Submit handlers (one per mode) ---

  const handleSessionSubmit = useCallback(async () => {
    const description = descriptionInputRef.current?.getValue() ?? '';
    const trimmedDescription = description.trim();
    if (!trimmedDescription || !agentProfileId) return;

    // Legacy mode: delegate to parent
    if (onCreateSession) {
      onCreateSession({ prompt: trimmedDescription, agentProfileId, executorId, environmentId });
      resetForm();
      onOpenChange(false);
      return;
    }

    if (!taskId) return;

    setIsCreatingSession(true);
    try {
      const client = getWebSocketClient();
      if (!client) throw new Error('WebSocket client not available');

      const response = await client.request<OrchestratorStartResponse>(
        'orchestrator.start',
        { task_id: taskId, agent_profile_id: agentProfileId, executor_id: executorId, prompt: trimmedDescription },
        15000,
      );

      resetForm();
      const newSessionId = response?.session_id;
      if (newSessionId) {
        router.push(linkToSession(newSessionId));
      } else {
        onOpenChange(false);
      }
    } catch (error) {
      toast({ title: 'Failed to create session', description: error instanceof Error ? error.message : 'An error occurred', variant: 'error' });
    } finally {
      setIsCreatingSession(false);
    }
  }, [agentProfileId, environmentId, executorId, onCreateSession, onOpenChange, resetForm, router, taskId, toast]);

  const performTaskUpdate = useCallback(async () => {
    if (!editingTask) return null;
    const trimmedTitle = taskName.trim();
    if (!trimmedTitle) return null;
    const description = descriptionInputRef.current?.getValue() ?? '';
    const trimmedDescription = description.trim();
    const repositoriesPayload = getRepositoriesPayload();

    const updatePayload: Parameters<typeof updateTask>[1] = {
      title: trimmedTitle,
      description: trimmedDescription,
      ...(repositoriesPayload.length > 0 && { repositories: repositoriesPayload }),
    };

    const updatedTask = await updateTask(editingTask.id, updatePayload);
    return { updatedTask, trimmedDescription };
  }, [editingTask, taskName, getRepositoriesPayload]);

  const handleEditSubmit = useCallback(async () => {
    setIsCreatingTask(true);
    try {
      const result = await performTaskUpdate();
      if (!result) return;
      const { updatedTask, trimmedDescription } = result;

      let taskSessionId: string | null = null;
      if (agentProfileId) {
        const autoStartStep = columns.find((column) => column.autoStartAgent);
        const client = getWebSocketClient();
        if (client) {
          try {
            const response = await client.request<OrchestratorStartResponse>(
              'orchestrator.start',
              {
                task_id: updatedTask.id,
                agent_profile_id: agentProfileId,
                executor_id: executorId,
                prompt: trimmedDescription || '',
                ...(autoStartStep && { workflow_step_id: autoStartStep.id }),
              },
              15000,
            );
            taskSessionId = response?.session_id ?? null;
          } catch (error) {
            console.error('[TaskCreateDialog] failed to start agent:', error);
          }
        }
      }

      onSuccess?.(updatedTask, 'edit', { taskSessionId });
    } catch (error) {
      toast({ title: 'Failed to update task', description: error instanceof Error ? error.message : 'An error occurred', variant: 'error' });
    } finally {
      resetForm();
      setIsCreatingTask(false);
      onOpenChange(false);
    }
  }, [performTaskUpdate, agentProfileId, executorId, columns, onSuccess, resetForm, onOpenChange, toast]);

  const handleUpdateWithoutAgent = useCallback(async () => {
    setIsCreatingTask(true);
    try {
      const result = await performTaskUpdate();
      if (!result) return;
      onSuccess?.(result.updatedTask, 'edit');
    } catch (error) {
      toast({ title: 'Failed to update task', description: error instanceof Error ? error.message : 'An error occurred', variant: 'error' });
    } finally {
      resetForm();
      setIsCreatingTask(false);
      onOpenChange(false);
    }
  }, [performTaskUpdate, onSuccess, onOpenChange, resetForm, toast]);

  const handleCreateSubmit = useCallback(async () => {
    const trimmedTitle = taskName.trim();
    if (!trimmedTitle) return;
    if (!workspaceId || !boardId) return;
    if (!repositoryId && !selectedLocalRepo) return;
    if (!agentProfileId) return;
    const columnId = editingTask?.workflowStepId ?? defaultColumnId;
    if (!columnId) return;

    const description = descriptionInputRef.current?.getValue() ?? '';
    const trimmedDescription = description.trim();
    const repositoriesPayload = getRepositoriesPayload();

    setIsCreatingTask(true);
    try {
      if (trimmedDescription) {
        // "Start" mode: create task + start agent in background, stay on board
        const autoStartStep = columns.find((column) => column.autoStartAgent);
        const targetColumnId = autoStartStep?.id ?? columnId;

        const taskResponse = await createTask({
          workspace_id: workspaceId,
          board_id: boardId,
          workflow_step_id: targetColumnId,
          title: trimmedTitle,
          description: trimmedDescription,
          repositories: repositoriesPayload,
          state: 'IN_PROGRESS',
          start_agent: true,
          agent_profile_id: agentProfileId || undefined,
          executor_id: executorId || undefined,
        });
        const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
        onSuccess?.(taskResponse, 'create', { taskSessionId: newSessionId });
        resetForm();
        onOpenChange(false);
      } else {
        // "Plan" mode: create task + session only (no agent), navigate to session
        const taskResponse = await createTask({
          workspace_id: workspaceId,
          board_id: boardId,
          workflow_step_id: columnId,
          title: trimmedTitle,
          description: '',
          repositories: repositoriesPayload,
          state: 'CREATED',
          prepare_session: true,
          agent_profile_id: agentProfileId || undefined,
          executor_id: executorId || undefined,
        });
        const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
        const newTaskId = taskResponse.id;
        onSuccess?.(taskResponse, 'create', { taskSessionId: newSessionId });
        resetForm();

        if (newSessionId) {
          setActiveDocument(newSessionId, { type: 'plan', taskId: newTaskId });
          useDockviewStore.getState().queuePanelAction({
            id: 'plan',
            component: 'plan',
            title: 'Plan',
            placement: 'right',
            referencePanel: 'chat',
          });
          setPlanMode(newSessionId, true);
          router.push(linkToSession(newSessionId));
        } else {
          onOpenChange(false);
        }
      }
    } catch (error) {
      toast({ title: 'Failed to create task', description: error instanceof Error ? error.message : 'An error occurred', variant: 'error' });
    } finally {
      setIsCreatingTask(false);
    }
  }, [
    taskName, workspaceId, boardId, repositoryId, selectedLocalRepo, agentProfileId, executorId,
    editingTask, defaultColumnId, columns, getRepositoriesPayload, onSuccess, resetForm, onOpenChange,
    setActiveDocument, setPlanMode, router, toast,
  ]);

  const handleCreateWithoutAgent = useCallback(async () => {
    const trimmedTitle = taskName.trim();
    const description = descriptionInputRef.current?.getValue() ?? '';
    const trimmedDescription = description.trim();
    if (!trimmedTitle || !trimmedDescription) return;
    if (!workspaceId || !boardId) return;
    if (!repositoryId && !selectedLocalRepo) return;
    if (!agentProfileId) return;
    const columnId = defaultColumnId;
    if (!columnId) return;

    setIsCreatingTask(true);
    try {
      const reposPayload = getRepositoriesPayload();
      const taskResponse = await createTask({
        workspace_id: workspaceId,
        board_id: boardId,
        workflow_step_id: columnId,
        title: trimmedTitle,
        description: trimmedDescription,
        repositories: reposPayload,
        state: 'CREATED',
        agent_profile_id: agentProfileId || undefined,
        executor_id: executorId || undefined,
      });
      onSuccess?.(taskResponse, 'create');
      resetForm();
      onOpenChange(false);
    } catch (error) {
      toast({ title: 'Failed to create task', description: error instanceof Error ? error.message : 'An error occurred', variant: 'error' });
    } finally {
      setIsCreatingTask(false);
    }
  }, [
    taskName, workspaceId, boardId, repositoryId, selectedLocalRepo,
    agentProfileId, defaultColumnId, executorId, getRepositoriesPayload,
    onSuccess, onOpenChange, resetForm, toast,
  ]);

  // Dispatch to the correct handler based on mode
  const handleSubmit = useCallback(async (e: FormEvent) => {
    e.preventDefault();
    if (isSessionMode) return handleSessionSubmit();
    if (isEditMode) return handleEditSubmit();
    return handleCreateSubmit();
  }, [isSessionMode, isEditMode, handleSessionSubmit, handleEditSubmit, handleCreateSubmit]);

  // Use keyboard shortcut hook for Cmd+Enter / Ctrl+Enter
  const handleKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, (event) => {
    handleSubmit(event as unknown as FormEvent);
  });

  const handleCancel = useCallback(() => {
    resetForm();
    onOpenChange(false);
  }, [resetForm, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full h-full max-w-full max-h-full rounded-none sm:w-[900px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col bg-card">
        <DialogHeader>
          {isCreateMode || isEditMode ? (
            <DialogTitle asChild>
              <div className="flex items-center gap-1 min-w-0 text-sm font-medium">
                <RepositorySelector
                  options={headerRepositoryOptions}
                  value={repositoryId || discoveredRepoPath}
                  onValueChange={handleRepositoryChange}
                  placeholder={
                    !workspaceId
                      ? 'Select workspace first'
                      : repositoriesLoading || discoverReposLoading
                        ? 'Loading...'
                        : 'Select repository'
                  }
                  searchPlaceholder="Search repositories..."
                  emptyMessage={
                    repositoriesLoading || discoverReposLoading
                      ? 'Loading repositories...'
                      : 'No repositories found.'
                  }
                  disabled={!workspaceId || repositoriesLoading || discoverReposLoading}
                  triggerClassName="w-auto text-sm"
                />
                <span className="text-muted-foreground mr-2">/</span>
                <InlineTaskName value={taskName} onChange={handleTaskNameChange} autoFocus={!isEditMode} />
              </div>
            </DialogTitle>
          ) : (
            <DialogTitle asChild>
              <div className="flex items-center gap-1 min-w-0 text-sm font-medium">
                {sessionRepoName && (
                  <>
                    <span className="truncate text-muted-foreground">{sessionRepoName}</span>
                    <span className="text-muted-foreground mx-0.5">/</span>
                  </>
                )}
                <span className="truncate">{initialValues?.title || 'Task'}</span>
                <span className="text-muted-foreground mx-0.5">/</span>
                <span className="text-muted-foreground whitespace-nowrap">new session</span>
              </div>
            </DialogTitle>
          )}
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4 overflow-hidden">
          <div className="flex-1 space-y-4 overflow-y-auto pr-1">
            <TaskFormInputs
              key={`${open}-${initialValues?.description ?? ''}`}
              isSessionMode={isSessionMode}
              autoFocus={isEditMode}
              initialDescription={initialValues?.description ?? ''}
              onDescriptionChange={setHasDescription}
              onKeyDown={handleKeyDown}
              descriptionValueRef={descriptionInputRef}
            />
            {!isSessionMode && (
              <div className="grid gap-4 grid-cols-1 sm:grid-cols-3">
                <div>
                  <BranchSelector
                    options={branchOptions}
                    value={branch}
                    onValueChange={handleBranchChange}
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
                      !hasRepositorySelection ||
                      (repositoryId && branchesLoading) ||
                      (!repositoryId && (localBranchesLoading || branchOptions.length === 0))
                    }
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
                    <AgentSelector
                      options={agentProfileOptions}
                      value={agentProfileId}
                      onValueChange={handleAgentProfileChange}
                      placeholder={
                        agentProfilesLoading
                          ? 'Loading agents...'
                          : agentProfiles.length === 0
                            ? 'No agents available'
                            : 'Select agent'
                      }
                      disabled={agentProfilesLoading || isCreatingSession}
                    />
                  )}
                </div>
                <div>
                  <ExecutorSelector
                    options={executorOptions}
                    value={executorId}
                    onValueChange={setExecutorId}
                    placeholder={
                      executorsLoading
                        ? 'Loading executors...'
                        : 'Select executor'
                    }
                    disabled={executorsLoading}
                  />
                </div>
              </div>
            )}

            {/* Agent Profile + Executor - shown in session mode */}
            {isSessionMode && (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <AgentSelector
                  options={agentProfileOptions}
                  value={agentProfileId}
                  onValueChange={handleAgentProfileChange}
                  placeholder={agentProfilesLoading ? 'Loading agent profiles...' : 'Select agent profile'}
                  disabled={agentProfilesLoading || isCreatingSession}
                />
                <ExecutorSelector
                  options={executorOptions}
                  value={executorId}
                  onValueChange={setExecutorId}
                  placeholder={executorsLoading ? 'Loading executors...' : 'Select executor'}
                  disabled={executorsLoading || isCreatingSession}
                />
              </div>
            )}

          </div>
          <DialogFooter className="border-t border-border pt-3 flex-col gap-3 sm:flex-row sm:gap-2">
            {!isSessionMode && executorHint && (
              <div className="flex flex-1 items-center gap-3 text-sm text-muted-foreground">
                <span className="text-xs text-muted-foreground">
                  {executorHint}
                </span>
              </div>
            )}
            <DialogClose asChild>
              <Button type="button" variant="outline" onClick={handleCancel} disabled={isCreatingSession || isCreatingTask} className="w-full h-10 border-0 cursor-pointer sm:w-auto sm:h-7 sm:border">
                Cancel
              </Button>
            </DialogClose>
            <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT}>
              {(isCreateMode && hasDescription) || (isEditMode && agentProfileId) ? (
                <div className="inline-flex rounded-md border border-border overflow-hidden sm:h-7 h-10">
                  <Button
                    type="submit"
                    variant="default"
                    className="rounded-none border-0 cursor-pointer gap-1.5 h-full"
                    disabled={isCreatingTask || !hasTitle || (!repositoryId && !selectedLocalRepo) || !branch || !agentProfileId}
                  >
                    {isCreatingTask ? <IconLoader2 className="h-3.5 w-3.5 animate-spin" /> : <IconSend className="h-3.5 w-3.5" />}
                    {isCreatingTask ? 'Starting...' : 'Start task'}
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        type="button"
                        variant="default"
                        className="rounded-none border-0 border-l border-primary-foreground/20 px-2 cursor-pointer h-full"
                        disabled={isCreatingTask || !hasTitle || (!repositoryId && !selectedLocalRepo) || !branch || !agentProfileId}
                      >
                        <IconChevronDown className="h-3.5 w-3.5" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-auto min-w-max">
                      <DropdownMenuItem
                        onClick={isEditMode ? handleUpdateWithoutAgent : handleCreateWithoutAgent}
                        className="cursor-pointer whitespace-nowrap focus:bg-muted/80 hover:bg-muted/80"
                      >
                        {isEditMode ? 'Update task' : 'Create without starting agent'}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              ) : (
                <Button
                  type="submit"
                  variant={'default'}
                  className={`w-full h-10 cursor-pointer sm:w-auto sm:h-7 gap-1.5 ${isCreateMode && !hasDescription ? 'bg-blue-600 border-blue-500 text-white hover:bg-blue-700 hover:text-white' : ''}`}
                  disabled={
                    isCreatingSession ||
                    isCreatingTask ||
                    (isSessionMode
                      ? !hasDescription || !agentProfileId
                      : !hasTitle || (!repositoryId && !selectedLocalRepo) || !branch)
                  }
                >
                  {isCreatingSession || isCreatingTask ? (
                    <>
                      <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
                      {isEditMode ? 'Updating...' : 'Starting...'}
                    </>
                  ) : isSessionMode ? (
                    'Create Session'
                  ) : isCreateMode ? (
                    <>
                      <IconFileInvoice className="h-3.5 w-3.5" />
                      Start Plan Mode
                    </>
                  ) : (
                    'Update task'
                  )}
                </Button>
              )}
            </KeyboardShortcutTooltip>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
