'use client';

import { useEffect, useRef, useState, FormEvent, memo, useMemo, useCallback } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { IconSettings, IconLoader2 } from '@tabler/icons-react';
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

interface TaskCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode?: 'task' | 'session';
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
  submitLabel?: string;
  taskId?: string | null;
  /** Whether to navigate to the new session after task creation. Defaults to true. */
  navigateOnSessionCreate?: boolean;
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

const RepositorySelector = memo(function RepositorySelector({
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
      className={disabled ? undefined : 'cursor-pointer'}
      triggerClassName={triggerClassName}
    />
  );
});

// Memoized text inputs to prevent re-rendering the entire dialog on every keystroke
type TaskFormInputsProps = {
  isSessionMode: boolean;
  initialTitle: string;
  initialDescription: string;
  onTitleChange: (hasContent: boolean) => void;
  onDescriptionChange: (hasContent: boolean) => void;
  onKeyDown: (e: React.KeyboardEvent) => void;
  titleValueRef: React.RefObject<{ getValue: () => string } | null>;
  descriptionValueRef: React.RefObject<{ getValue: () => string } | null>;
};

const TaskFormInputs = memo(function TaskFormInputs({
  isSessionMode,
  initialTitle,
  initialDescription,
  onTitleChange,
  onDescriptionChange,
  onKeyDown,
  titleValueRef,
  descriptionValueRef,
}: TaskFormInputsProps) {
  const [title, setTitle] = useState(initialTitle);
  const [description, setDescription] = useState(initialDescription);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Expose getValue methods via refs passed from parent
  // This is an imperative handle pattern - refs are intentionally mutated
  useEffect(() => {
    const ref = titleValueRef as React.MutableRefObject<{ getValue: () => string } | null>;
    if (ref) {
      ref.current = { getValue: () => title };
    }
  }, [title, titleValueRef]);

  useEffect(() => {
    const ref = descriptionValueRef as React.MutableRefObject<{ getValue: () => string } | null>;
    if (ref) {
      // eslint-disable-next-line react-hooks/immutability
      ref.current = { getValue: () => description };
    }
  }, [description, descriptionValueRef]);

  // Auto-resize textarea
  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.style.height = 'auto';
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [description]);

  const handleTitleChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value;
    const hadContent = title.trim().length > 0;
    const hasContent = newValue.trim().length > 0;
    setTitle(newValue);
    if (hadContent !== hasContent) {
      onTitleChange(hasContent);
    }
  }, [title, onTitleChange]);

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
    <>
      {!isSessionMode && (
        <div>
          <Input
            autoFocus
            required
            placeholder="Enter task title..."
            value={title}
            onChange={handleTitleChange}
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
          ref={textareaRef}
          placeholder={isSessionMode ? 'Describe what you want the agent to do...' : 'Write a prompt for the agent...'}
          value={description}
          onChange={handleDescriptionChange}
          onKeyDown={onKeyDown}
          rows={2}
          className={isSessionMode ? 'min-h-[120px] max-h-[240px] resize-none overflow-auto mt-1.5' : 'min-h-[96px] max-h-[240px] resize-y overflow-auto'}
          required={isSessionMode}
        />
      </div>
    </>
  );
});

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
  taskId = null,
  navigateOnSessionCreate = true,
}: TaskCreateDialogProps) {
  const router = useRouter();
  const isSessionMode = mode === 'session';
  const isEditMode = submitLabel !== 'Create';
  const [isCreatingSession, setIsCreatingSession] = useState(false);
  const [isCreatingTask, setIsCreatingTask] = useState(false);
  // Track only whether inputs have content (not the actual values) to minimize re-renders
  const [hasTitle, setHasTitle] = useState(Boolean(initialValues?.title?.trim()));
  const [hasDescription, setHasDescription] = useState(Boolean(initialValues?.description?.trim()));
  // Refs to get actual values when needed
  const titleInputRef = useRef<{ getValue: () => string } | null>(null);
  const descriptionInputRef = useRef<{ getValue: () => string } | null>(null);
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
    setHasTitle(Boolean(initialValues?.title?.trim()));
    setHasDescription(Boolean(initialValues?.description?.trim()));
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
    if (!repositoryId) return;
    if (branch) return;

    // Priority 1: Last used branch
    const lastUsedBranch = getLocalStorage<string | null>(STORAGE_KEYS.LAST_BRANCH, null);
    if (lastUsedBranch && branches.some((b: Branch) => {
      const displayName = b.type === 'remote' && b.remote ? `${b.remote}/${b.name}` : b.name;
      return displayName === lastUsedBranch;
    })) {
      setBranch(lastUsedBranch);
      return;
    }

    // Priority 2: Preferred branch (main/master)
    const preferredBranch = selectPreferredBranch(branches);
    if (preferredBranch) {
      setBranch(preferredBranch);
    }
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
            <span className="truncate" title={displayName}>
              {displayName}
            </span>
            <Badge variant={branchObj.type === 'local' ? 'default' : 'secondary'} className="text-xs">
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
            <span className="truncate">{agentLabel}</span>
            {profileLabel ? (
              <Badge variant="secondary" className="text-xs">
                {profileLabel}
              </Badge>
            ) : null}
          </span>
        ),
      };
    });
  }, [agentProfiles]);

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

    // Priority 1: Last used branch
    const lastUsedBranch = getLocalStorage<string | null>(STORAGE_KEYS.LAST_BRANCH, null);
    if (lastUsedBranch && localBranches.some((b: Branch) => {
      const displayName = b.type === 'remote' && b.remote ? `${b.remote}/${b.name}` : b.name;
      return displayName === lastUsedBranch;
    })) {
      setBranch(lastUsedBranch);
      return;
    }

    // Priority 2: Preferred branch (main/master)
    const preferredBranch = selectPreferredBranch(localBranches);
    if (preferredBranch) {
      setBranch(preferredBranch);
    }
  }, [branch, localBranches, repositoryId]);

  useEffect(() => {
    if (!open || agentProfileId || agentProfiles.length === 0) return;
    if (!isSessionMode && !startAgent) return;
    if (isEditMode && agentProfiles.length > 1) return;

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
  }, [open, isEditMode, agentProfileId, agentProfiles, workspaceDefaults, isSessionMode, startAgent]);

  useEffect(() => {
    if (!open || isEditMode || environmentId || environments.length === 0) return;
    if (!isSessionMode && !startAgent) return;
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
  }, [open, isEditMode, environmentId, environments, workspaceDefaults, isSessionMode, startAgent]);

  useEffect(() => {
    if (!open || isEditMode || executorId || executors.length === 0) return;
    if (!isSessionMode && !startAgent) return;
    const defaultExecutorId = workspaceDefaults?.default_executor_id ?? null;
    if (defaultExecutorId && executors.some((executor: Executor) => executor.id === defaultExecutorId)) {
      setExecutorId(defaultExecutorId);
      return;
    }
    const localExecutor = executors.find(
      (executor: Executor) => executor.type === DEFAULT_LOCAL_EXECUTOR_TYPE
    );
    setExecutorId(localExecutor?.id ?? executors[0].id);
  }, [open, isEditMode, executorId, executors, workspaceDefaults, isSessionMode, startAgent]);

  // Use keyboard shortcut hook for Cmd+Enter / Ctrl+Enter
  const handleKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, (event) => {
    handleSubmit(event as unknown as FormEvent);
  });

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();

    // Get values from refs
    const title = titleInputRef.current?.getValue() ?? '';
    const description = descriptionInputRef.current?.getValue() ?? '';

    // Session mode - create a new session
    if (isSessionMode) {
      const trimmedDescription = description.trim();
      if (!trimmedDescription || !agentProfileId) return;

      // If onCreateSession is provided (legacy mode), delegate to parent
      if (onCreateSession) {
        onCreateSession({
          prompt: trimmedDescription,
          agentProfileId,
          executorId,
          environmentId,
        });
        // Reset form
        setHasDescription(false);
        setAgentProfileId('');
        setExecutorId('');
        setEnvironmentId('');
        setShowAdvancedSettings(false);
        onOpenChange(false);
        return;
      }

      // Handle session creation directly with spinner and navigation
      if (!taskId) return;

      setIsCreatingSession(true);
      try {
        const client = getWebSocketClient();
        if (!client) {
          throw new Error('WebSocket client not available');
        }

        interface StartResponse {
          success: boolean;
          task_id: string;
          agent_instance_id: string;
          session_id?: string;
          state: string;
        }

        const response = await client.request<StartResponse>(
          'orchestrator.start',
          {
            task_id: taskId,
            agent_profile_id: agentProfileId,
            executor_id: executorId,
            prompt: trimmedDescription,
          },
          15000
        );

        const newSessionId = response?.session_id;

        // Reset form
        setHasDescription(false);
        setAgentProfileId('');
        setExecutorId('');
        setEnvironmentId('');
        setShowAdvancedSettings(false);

        // Navigate to the new session (this will unmount the dialog)
        if (newSessionId) {
          router.push(linkToSession(newSessionId));
        } else {
          onOpenChange(false);
        }
      } catch (error) {
        toast({
          title: 'Failed to create session',
          description: error instanceof Error ? error.message : 'An error occurred while creating the session',
          variant: 'error',
        });
      } finally {
        setIsCreatingSession(false);
      }
      return;
    }

    // Task mode - create or edit task
    const trimmedTitle = title.trim();
    if (!trimmedTitle) return;
    if (isEditMode && editingTask) {
      try {
        const updatePayload: Parameters<typeof updateTask>[1] = {
          title: trimmedTitle,
          description: description.trim(),
        };

        const needsRepository = startAgent && !editingTask.repositoryId;
        if (repositoryId) {
          updatePayload.repositories = [{ repository_id: repositoryId, base_branch: branch || undefined }];
        } else if (selectedLocalRepo && needsRepository) {
          console.warn('[TaskCreateDialog] Local repo selection in edit mode not fully supported');
        }

        const updatedTask = await updateTask(editingTask.id, updatePayload);

        let taskSessionId: string | null = null;
        if (startAgent && agentProfileId) {
          const client = getWebSocketClient();
          if (client) {
            try {
              interface StartResponse {
                success: boolean;
                task_id: string;
                agent_instance_id: string;
                session_id?: string;
                state: string;
              }
              const response = await client.request<StartResponse>(
                'orchestrator.start',
                {
                  task_id: editingTask.id,
                  agent_profile_id: agentProfileId,
                  prompt: description.trim() || '',
                },
                15000
              );
              taskSessionId = response?.session_id ?? null;
            } catch (error) {
              console.error('[TaskCreateDialog] failed to start agent:', error);
            }
          }
        }

        onSuccess?.(updatedTask, 'edit', { taskSessionId });
      } finally {
        setHasTitle(false);
        setHasDescription(false);
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
    const columnId = editingTask?.workflowStepId ?? defaultColumnId;
    if (!columnId) return;
    let targetColumnId = columnId;
    let targetState: Task['state'] = 'CREATED';
    if (startAgent && !isEditMode) {
      // Find the first step that has auto_start_agent enabled
      const autoStartStep = columns.find((column) => column.autoStartAgent);
      targetColumnId = autoStartStep?.id ?? columnId;
      targetState = 'IN_PROGRESS';
    }

    setIsCreatingTask(true);
    try {
      const taskResponse = await createTask({
        workspace_id: workspaceId,
        board_id: boardId,
        workflow_step_id: targetColumnId,
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
        executor_id: startAgent ? executorId || undefined : undefined,
      });
      // Try session_id first, then primary_session_id as fallback
      const newSessionId = taskResponse.session_id ?? taskResponse.primary_session_id ?? null;
      onSuccess?.(taskResponse, 'create', {
        taskSessionId: newSessionId,
      });

      // Reset form and close dialog
      setHasTitle(false);
      setHasDescription(false);
      setRepositoryId('');
      setBranch('');
      setStartAgent(!isEditMode);
      setAgentProfileId('');
      setEnvironmentId('');
      setExecutorId('');

      // Navigate to the new session if one was created and navigation is enabled
      if (newSessionId && navigateOnSessionCreate) {
        router.push(linkToSession(newSessionId));
      } else {
        onOpenChange(false);
      }
    } catch (error) {
      toast({
        title: 'Failed to create task',
        description: error instanceof Error ? error.message : 'An error occurred while creating the task',
        variant: 'error',
      });
    } finally {
      setIsCreatingTask(false);
    }
  };

  const handleCancel = () => {
    setHasTitle(false);
    setHasDescription(false);
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
      <DialogContent className="w-full h-full max-w-full max-h-full rounded-none sm:w-[900px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col bg-card">
        <DialogHeader>
          <DialogTitle>
            {isSessionMode ? 'Create New Session' : submitLabel === 'Create' ? 'Create Task' : 'Edit Task'}
          </DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4 overflow-hidden">
          <div className="flex-1 space-y-4 overflow-y-auto pr-1">
            <TaskFormInputs
              key={`${open}-${initialValues?.title ?? ''}-${initialValues?.description ?? ''}`}
              isSessionMode={isSessionMode}
              initialTitle={initialValues?.title ?? ''}
              initialDescription={initialValues?.description ?? ''}
              onTitleChange={setHasTitle}
              onDescriptionChange={setHasDescription}
              onKeyDown={handleKeyDown}
              titleValueRef={titleInputRef}
              descriptionValueRef={descriptionInputRef}
            />
            {!isSessionMode && (
              <div className="grid gap-4 grid-cols-1 sm:grid-cols-3">
                <div>
                  <RepositorySelector
                    options={repositoryOptions}
                    value={repositoryId || discoveredRepoPath}
                    onValueChange={handleRepositoryChange}
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
                )}
              </div>
            )}

            {/* Agent Profile - shown in session mode */}
            {isSessionMode && (
              <div className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-3">
                  <AgentSelector
                    options={agentProfileOptions}
                    value={agentProfileId}
                    onValueChange={handleAgentProfileChange}
                    placeholder={agentProfilesLoading ? 'Loading agent profiles...' : 'Select agent profile'}
                    disabled={agentProfilesLoading || isCreatingSession}
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
                        {environments.map((env: Environment) => (
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
                        {executors.map((executor: Executor) => (
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
                      {environments.map((environment: Environment) => (
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
                      {executors.map((executor: Executor) => (
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
          <DialogFooter className="border-t border-border pt-3 flex-col gap-3 sm:flex-row sm:gap-2">
            {!isSessionMode && (
              <div className="flex flex-1 items-center gap-3 text-sm text-muted-foreground">
                <div className="flex items-center gap-2">
                  <input
                    id="start-agent"
                    type="checkbox"
                    checked={startAgent}
                    onChange={(e) => setStartAgent(e.target.checked)}
                    className="h-4 w-4 rounded border border-input text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                  <TooltipProvider>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Label htmlFor="start-agent" className="cursor-pointer">
                          Start Agent
                        </Label>
                      </TooltipTrigger>
                      <TooltipContent>
                        {isEditMode
                          ? 'Start the agent after saving changes.'
                          : 'Start the agent on task creation.'}
                      </TooltipContent>
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
              <Button type="button" variant="outline" onClick={handleCancel} disabled={isCreatingSession || isCreatingTask} className="w-full h-10 border-0 sm:w-auto sm:h-7 sm:border">
                Cancel
              </Button>
            </DialogClose>
            <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT}>
              <Button
                type="submit"
                className="w-full h-10 sm:w-auto sm:h-7"
                disabled={
                  isCreatingSession ||
                  isCreatingTask ||
                  (isSessionMode
                    ? !hasDescription || !agentProfileId
                    : !hasTitle ||
                    !hasDescription ||
                    (!isEditMode && !repositoryId && !selectedLocalRepo) ||
                    (startAgent && !agentProfileId) ||
                    (isEditMode && startAgent && !editingTask?.repositoryId && !repositoryId))
                }
              >
                {isCreatingSession || isCreatingTask ? (
                  <>
                    <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
                    Creating...
                  </>
                ) : isSessionMode ? (
                  'Create Session'
                ) : (
                  submitLabel
                )}
              </Button>
            </KeyboardShortcutTooltip>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
