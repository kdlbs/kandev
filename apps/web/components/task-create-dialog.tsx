'use client';

import { useEffect, useRef, useState, FormEvent, useMemo, useCallback } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@kandev/ui/dialog';
import type { LocalRepository, Task, Workspace, Repository, Environment, Executor, Branch } from '@/lib/types/http';
import type { AgentProfileOption } from '@/lib/state/slices';
import {
  DEFAULT_LOCAL_ENVIRONMENT_KIND,
  DEFAULT_LOCAL_EXECUTOR_TYPE,
  selectPreferredBranch,
} from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import { useRepositories } from '@/hooks/domains/workspace/use-repositories';
import { useRepositoryBranches } from '@/hooks/domains/workspace/use-repository-branches';
import { useSettingsData } from '@/hooks/domains/settings/use-settings-data';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { useKeyboardShortcutHandler } from '@/hooks/use-keyboard-shortcut';
import { TaskCreateDialogFooter } from '@/components/task-create-dialog-footer';
import { CreateEditSelectors, SessionSelectors, WorkflowSection } from '@/components/task-create-dialog-form-body';
import { useRepositoryOptions, useBranchOptions, useAgentProfileOptions, useExecutorOptions, useExecutorHint } from '@/components/task-create-dialog-options';
import { RepositorySelector, BranchSelector, AgentSelector, ExecutorSelector, InlineTaskName, TaskFormInputs } from '@/components/task-create-dialog-selectors';
import { useTaskSubmitHandlers } from '@/components/task-create-dialog-submit';
import { useToast } from '@/components/toast-provider';
import { discoverRepositoriesAction, listLocalRepositoryBranchesAction } from '@/app/actions/workspaces';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { STORAGE_KEYS } from '@/lib/settings/constants';
import { listWorkflowSteps } from '@/lib/api/domains/workflow-api';

interface TaskCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode?: 'create' | 'edit' | 'session';
  workspaceId: string | null;
  workflowId: string | null;
  defaultStepId: string | null;
  steps: Array<{ id: string; title: string; events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }>; on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }> } }>;
  editingTask?: { id: string; title: string; description?: string; workflowStepId: string; state?: Task['state']; repositoryId?: string } | null;
  onSuccess?: (task: Task, mode: 'create' | 'edit', meta?: { taskSessionId?: string | null }) => void;
  onCreateSession?: (data: { prompt: string; agentProfileId: string; executorId: string; environmentId: string }) => void;
  initialValues?: { title: string; description?: string; repositoryId?: string; branch?: string; state?: Task['state'] };
  taskId?: string | null;
}

type StepType = { id: string; title: string; events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }>; on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }> } };

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
  if (preferredBranch) setBranch(preferredBranch);
}

function useDialogFormState(open: boolean, workspaceId: string | null, workflowId: string | null, initialValues?: TaskCreateDialogProps['initialValues']) {
  const [taskName, setTaskName] = useState('');
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
  const [localBranches, setLocalBranches] = useState<Branch[]>([]);
  const [localBranchesLoading, setLocalBranchesLoading] = useState(false);
  const [discoverReposLoading, setDiscoverReposLoading] = useState(false);
  const [discoverReposLoaded, setDiscoverReposLoaded] = useState(false);
  const [selectedWorkflowId, setSelectedWorkflowId] = useState(workflowId);
  const [fetchedSteps, setFetchedSteps] = useState<StepType[] | null>(null);
  const [isCreatingSession, setIsCreatingSession] = useState(false);
  const [isCreatingTask, setIsCreatingTask] = useState(false);

  useEffect(() => {
    if (!open) return;
    const name = initialValues?.title || '';
    void Promise.resolve().then(() => {
      setTaskName(name);
      setHasTitle(name.trim().length > 0);
      setHasDescription(Boolean(initialValues?.description?.trim()));
      setRepositoryId(initialValues?.repositoryId ?? '');
      setBranch(initialValues?.branch ?? '');
      setAgentProfileId('');
      setEnvironmentId('');
      setExecutorId('');
      setSelectedWorkflowId(workflowId);
      setFetchedSteps(null);
    });
  }, [initialValues?.branch, initialValues?.description, initialValues?.repositoryId, initialValues?.title, open, workflowId]);

  useEffect(() => {
    if (!open) return;
    void Promise.resolve().then(() => {
      setDiscoveredRepositories([]);
      setDiscoveredRepoPath('');
      setSelectedLocalRepo(null);
      setLocalBranches([]);
      setDiscoverReposLoaded(false);
    });
  }, [open, workspaceId]);

  return {
    taskName, setTaskName, hasTitle, setHasTitle, hasDescription, setHasDescription, descriptionInputRef,
    repositoryId, setRepositoryId, branch, setBranch, agentProfileId, setAgentProfileId,
    environmentId, setEnvironmentId, executorId, setExecutorId,
    discoveredRepositories, setDiscoveredRepositories, discoveredRepoPath, setDiscoveredRepoPath,
    selectedLocalRepo, setSelectedLocalRepo, localBranches, setLocalBranches,
    localBranchesLoading, setLocalBranchesLoading, discoverReposLoading, setDiscoverReposLoading,
    discoverReposLoaded, setDiscoverReposLoaded, selectedWorkflowId, setSelectedWorkflowId,
    fetchedSteps, setFetchedSteps, isCreatingSession, setIsCreatingSession, isCreatingTask, setIsCreatingTask,
  };
}

type DialogFormState = ReturnType<typeof useDialogFormState>;

function useWorkflowStepsEffect(fs: DialogFormState, workflowId: string | null) {
  const { selectedWorkflowId, setFetchedSteps } = fs;
  useEffect(() => {
    if (!selectedWorkflowId || selectedWorkflowId === workflowId) {
      void Promise.resolve().then(() => setFetchedSteps(null));
      return;
    }
    let cancelled = false;
    listWorkflowSteps(selectedWorkflowId).then((response) => {
      if (cancelled) return;
      const sorted = [...response.steps].sort((a, b) => a.position - b.position);
      setFetchedSteps(sorted.map((s) => ({ id: s.id, title: s.name, events: s.events })));
    }).catch(() => { if (!cancelled) setFetchedSteps(null); });
    return () => { cancelled = true; };
  }, [selectedWorkflowId, workflowId, setFetchedSteps]);
}

function useRepositoryAutoSelectEffect(fs: DialogFormState, open: boolean, workspaceId: string | null, repositories: Repository[]) {
  const { repositoryId, selectedLocalRepo, setRepositoryId } = fs;
  useEffect(() => {
    if (!open || !workspaceId || repositoryId || selectedLocalRepo) return;
    const lastUsedRepoId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null);
    if (lastUsedRepoId && repositories.some((r: Repository) => r.id === lastUsedRepoId)) {
      void Promise.resolve().then(() => setRepositoryId(lastUsedRepoId)); return;
    }
    if (repositories.length === 1) void Promise.resolve().then(() => setRepositoryId(repositories[0].id));
  }, [open, repositories, repositoryId, selectedLocalRepo, workspaceId, setRepositoryId]);
}

function useDiscoverReposEffect(fs: DialogFormState, open: boolean, workspaceId: string | null, repositoriesLoading: boolean, toast: ReturnType<typeof useToast>['toast']) {
  const { discoverReposLoaded, discoverReposLoading, setDiscoveredRepositories, setDiscoverReposLoading, setDiscoverReposLoaded } = fs;
  useEffect(() => {
    if (!open || !workspaceId || repositoriesLoading || discoverReposLoaded || discoverReposLoading) return;
    void Promise.resolve()
      .then(() => setDiscoverReposLoading(true))
      .then(() => discoverRepositoriesAction(workspaceId))
      .then((r) => { setDiscoveredRepositories(r.repositories); })
      .catch((e) => { toast({ title: 'Failed to discover repositories', description: e instanceof Error ? e.message : 'Request failed', variant: 'error' }); setDiscoveredRepositories([]); })
      .finally(() => { setDiscoverReposLoading(false); setDiscoverReposLoaded(true); });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [discoverReposLoaded, discoverReposLoading, open, fs.discoveredRepositories.length, repositoriesLoading, toast, workspaceId]);
}

function useLocalBranchesEffect(fs: DialogFormState, open: boolean, workspaceId: string | null, toast: ReturnType<typeof useToast>['toast']) {
  const { selectedLocalRepo, setLocalBranches, setLocalBranchesLoading } = fs;
  useEffect(() => {
    if (!open || !workspaceId || !selectedLocalRepo) return;
    const repoPath = selectedLocalRepo.path;
    void Promise.resolve()
      .then(() => setLocalBranchesLoading(true))
      .then(() => listLocalRepositoryBranchesAction(workspaceId, repoPath))
      .then((r) => { setLocalBranches(r.branches); })
      .catch((e) => { toast({ title: 'Failed to load branches', description: e instanceof Error ? e.message : 'Request failed', variant: 'error' }); setLocalBranches([]); })
      .finally(() => { setLocalBranchesLoading(false); });
  }, [open, selectedLocalRepo, toast, workspaceId, setLocalBranches, setLocalBranchesLoading]);
}

type StoreSelections = { agentProfiles: AgentProfileOption[]; environments: Environment[]; executors: Executor[]; workspaceDefaults: Workspace | null | undefined };

function useDefaultSelectionsEffect(fs: DialogFormState, open: boolean, sel: StoreSelections) {
  const { agentProfiles, environments, executors, workspaceDefaults } = sel;
  const { agentProfileId, environmentId, executorId, setAgentProfileId, setEnvironmentId, setExecutorId } = fs;
  useEffect(() => {
    if (!open || agentProfileId || agentProfiles.length === 0) return;
    const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, null);
    if (lastId && agentProfiles.some((p: AgentProfileOption) => p.id === lastId)) { void Promise.resolve().then(() => setAgentProfileId(lastId)); return; }
    const defId = workspaceDefaults?.default_agent_profile_id ?? null;
    if (defId && agentProfiles.some((p: AgentProfileOption) => p.id === defId)) { void Promise.resolve().then(() => setAgentProfileId(defId)); return; }
    void Promise.resolve().then(() => setAgentProfileId(agentProfiles[0].id));
  }, [open, agentProfileId, agentProfiles, workspaceDefaults, setAgentProfileId]);

  useEffect(() => {
    if (!open || environmentId || environments.length === 0) return;
    const defId = workspaceDefaults?.default_environment_id ?? null;
    if (defId && environments.some((e: Environment) => e.id === defId)) { void Promise.resolve().then(() => setEnvironmentId(defId)); return; }
    const local = environments.find((e: Environment) => e.kind === DEFAULT_LOCAL_ENVIRONMENT_KIND);
    void Promise.resolve().then(() => setEnvironmentId(local?.id ?? environments[0].id));
  }, [open, environmentId, environments, workspaceDefaults, setEnvironmentId]);

  useEffect(() => {
    if (!open || executorId || executors.length === 0) return;
    const defId = workspaceDefaults?.default_executor_id ?? null;
    if (defId && executors.some((e: Executor) => e.id === defId)) { void Promise.resolve().then(() => setExecutorId(defId)); return; }
    const local = executors.find((e: Executor) => e.type === DEFAULT_LOCAL_EXECUTOR_TYPE);
    void Promise.resolve().then(() => setExecutorId(local?.id ?? executors[0].id));
  }, [open, executorId, executors, workspaceDefaults, setExecutorId]);
}

function useBranchAutoSelectEffect(fs: DialogFormState, branches: Branch[]) {
  const { repositoryId, branch, localBranches, setBranch } = fs;
  useEffect(() => { if (!repositoryId || branch) return; autoSelectBranch(branches, setBranch); }, [branch, branches, repositoryId, setBranch]);
  useEffect(() => { if (repositoryId || localBranches.length === 0 || branch) return; autoSelectBranch(localBranches, setBranch); }, [branch, localBranches, repositoryId, setBranch]);
}

type DialogHeaderContentProps = {
  isCreateMode: boolean; isEditMode: boolean; isTaskStarted: boolean;
  sessionRepoName?: string; initialTitle?: string; taskName: string;
  repositoryId: string; discoveredRepoPath: string; workspaceId: string | null;
  repositoriesLoading: boolean; discoverReposLoading: boolean;
  headerRepositoryOptions: ReturnType<typeof useRepositoryOptions>['headerRepositoryOptions'];
  onRepositoryChange: (v: string) => void; onTaskNameChange: (v: string) => void;
};

function getRepositoryPlaceholder(workspaceId: string | null, repositoriesLoading: boolean, discoverReposLoading: boolean) {
  if (!workspaceId) return 'Select workspace first';
  if (repositoriesLoading || discoverReposLoading) return 'Loading...';
  return 'Select repository';
}

function DialogHeaderContent({ isCreateMode, isEditMode, isTaskStarted, sessionRepoName, initialTitle, taskName, repositoryId, discoveredRepoPath, workspaceId, repositoriesLoading, discoverReposLoading, headerRepositoryOptions, onRepositoryChange, onTaskNameChange }: DialogHeaderContentProps) {
  if (isCreateMode || isEditMode) {
    return (
      <DialogTitle asChild>
        <div className="flex items-center gap-1 min-w-0 text-sm font-medium">
          <RepositorySelector
            options={headerRepositoryOptions}
            value={repositoryId || discoveredRepoPath}
            onValueChange={onRepositoryChange}
            placeholder={getRepositoryPlaceholder(workspaceId, repositoriesLoading, discoverReposLoading)}
            searchPlaceholder="Search repositories..."
            emptyMessage={(repositoriesLoading || discoverReposLoading) ? 'Loading repositories...' : 'No repositories found.'}
            disabled={isTaskStarted || !workspaceId || repositoriesLoading || discoverReposLoading}
            triggerClassName="w-auto text-sm"
          />
          <span className="text-muted-foreground mr-2">/</span>
          <InlineTaskName value={taskName} onChange={onTaskNameChange} autoFocus={!isEditMode} />
        </div>
      </DialogTitle>
    );
  }
  return (
    <DialogTitle asChild>
      <div className="flex items-center gap-1 min-w-0 text-sm font-medium">
        {sessionRepoName && <><span className="truncate text-muted-foreground">{sessionRepoName}</span><span className="text-muted-foreground mx-0.5">/</span></>}
        <span className="truncate">{initialTitle || 'Task'}</span>
        <span className="text-muted-foreground mx-0.5">/</span>
        <span className="text-muted-foreground whitespace-nowrap">new session</span>
      </div>
    </DialogTitle>
  );
}

type DialogFormBodyProps = {
  open: boolean; isSessionMode: boolean; isCreateMode: boolean; isTaskStarted: boolean;
  isPassthroughProfile: boolean; initialDescription: string; hasDescription: boolean;
  branchOptions: ReturnType<typeof useBranchOptions>; branchesLoading: boolean;
  agentProfileOptions: ReturnType<typeof useAgentProfileOptions>;
  executorOptions: ReturnType<typeof useExecutorOptions>;
  agentProfiles: AgentProfileOption[]; agentProfilesLoading: boolean; executorsLoading: boolean;
  isCreatingSession: boolean; workflows: unknown[]; snapshots: unknown;
  effectiveWorkflowId: string | null; fs: DialogFormState;
  handleKeyDown: ReturnType<typeof useKeyboardShortcutHandler>;
  onBranchChange: (v: string) => void; onAgentProfileChange: (v: string) => void; onWorkflowChange: (v: string) => void;
  hasRepositorySelection: boolean;
};

function DialogFormBody({ open, isSessionMode, isCreateMode, isTaskStarted, isPassthroughProfile, initialDescription, hasDescription, branchOptions, branchesLoading, agentProfileOptions, executorOptions, agentProfiles, agentProfilesLoading, executorsLoading, isCreatingSession, workflows, snapshots, effectiveWorkflowId, fs, handleKeyDown, onBranchChange, onAgentProfileChange, onWorkflowChange, hasRepositorySelection }: DialogFormBodyProps) {
  return (
    <div className="flex-1 space-y-4 overflow-y-auto pr-1">
      <TaskFormInputs
        key={`${open}-${initialDescription}`}
        isSessionMode={isSessionMode} autoFocus={isTaskStarted ? false : true}
        initialDescription={initialDescription} onDescriptionChange={fs.setHasDescription}
        onKeyDown={handleKeyDown} descriptionValueRef={fs.descriptionInputRef}
        disabled={isTaskStarted || isPassthroughProfile}
        placeholder={isPassthroughProfile ? 'Sending a prompt is not supported in passthrough mode' : undefined}
      />
      {isPassthroughProfile && hasDescription && (
        <p className="text-xs text-amber-500">Prompt will be ignored — passthrough sessions don&apos;t support sending a prompt on start.</p>
      )}
      {!isSessionMode && (
        <CreateEditSelectors
          isTaskStarted={isTaskStarted} hasRepositorySelection={hasRepositorySelection}
          repositoryId={fs.repositoryId} branchOptions={branchOptions} branch={fs.branch}
          onBranchChange={onBranchChange} branchesLoading={branchesLoading}
          localBranchesLoading={fs.localBranchesLoading} agentProfiles={agentProfiles}
          agentProfilesLoading={agentProfilesLoading} agentProfileOptions={agentProfileOptions}
          agentProfileId={fs.agentProfileId} onAgentProfileChange={onAgentProfileChange}
          isCreatingSession={isCreatingSession} executorOptions={executorOptions}
          executorId={fs.executorId} onExecutorChange={fs.setExecutorId}
          executorsLoading={executorsLoading} BranchSelectorComponent={BranchSelector}
          AgentSelectorComponent={AgentSelector} ExecutorSelectorComponent={ExecutorSelector}
        />
      )}
      <WorkflowSection
        isCreateMode={isCreateMode} isTaskStarted={isTaskStarted} workflows={workflows as Parameters<typeof WorkflowSection>[0]['workflows']}
        snapshots={snapshots as Parameters<typeof WorkflowSection>[0]['snapshots']} effectiveWorkflowId={effectiveWorkflowId}
        onWorkflowChange={onWorkflowChange}
      />
      {isSessionMode && (
        <SessionSelectors
          agentProfileOptions={agentProfileOptions} agentProfileId={fs.agentProfileId}
          onAgentProfileChange={onAgentProfileChange} agentProfilesLoading={agentProfilesLoading}
          isCreatingSession={isCreatingSession} executorOptions={executorOptions}
          executorId={fs.executorId} onExecutorChange={fs.setExecutorId}
          executorsLoading={executorsLoading} AgentSelectorComponent={AgentSelector}
          ExecutorSelectorComponent={ExecutorSelector}
        />
      )}
    </div>
  );
}

function useDialogHandlers(fs: DialogFormState, repositories: Repository[]) {
  const handleSelectLocalRepository = useCallback((path: string) => {
    fs.setDiscoveredRepoPath(path);
    fs.setSelectedLocalRepo(fs.discoveredRepositories.find((r) => r.path === path) ?? null);
    fs.setRepositoryId('');
    fs.setBranch('');
    fs.setLocalBranches([]);
  }, [fs]);

  const handleRepositoryChange = useCallback((value: string) => {
    if (repositories.find((r: Repository) => r.id === value)) {
      fs.setRepositoryId(value);
      setLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID, value);
      fs.setDiscoveredRepoPath('');
      fs.setSelectedLocalRepo(null);
      fs.setLocalBranches([]);
      fs.setBranch('');
      return;
    }
    handleSelectLocalRepository(value);
  }, [repositories, fs, handleSelectLocalRepository]);

  const handleAgentProfileChange = useCallback((value: string) => { fs.setAgentProfileId(value); setLocalStorage(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, value); }, [fs]);
  const handleTaskNameChange = useCallback((value: string) => { fs.setTaskName(value); fs.setHasTitle(value.trim().length > 0); }, [fs]);
  const handleBranchChange = useCallback((value: string) => { fs.setBranch(value); setLocalStorage(STORAGE_KEYS.LAST_BRANCH, value); }, [fs]);
  const handleWorkflowChange = useCallback((value: string) => { fs.setSelectedWorkflowId(value); }, [fs]);

  return { handleRepositoryChange, handleAgentProfileChange, handleTaskNameChange, handleBranchChange, handleWorkflowChange };
}

function computePassthroughProfile(agentProfileId: string, agentProfiles: AgentProfileOption[]) {
  if (!agentProfileId) return false;
  return agentProfiles.find((p: AgentProfileOption) => p.id === agentProfileId)?.cli_passthrough === true;
}

function computeEffectiveStepId(selectedWorkflowId: string | null, workflowId: string | null, fetchedSteps: StepType[] | null, defaultStepId: string | null) {
  return (selectedWorkflowId && selectedWorkflowId !== workflowId && fetchedSteps)
    ? fetchedSteps[0]?.id ?? null : defaultStepId;
}

function computeIsTaskStarted(isEditMode: boolean, editingTask?: TaskCreateDialogProps['editingTask']) {
  if (!isEditMode || !editingTask?.state) return false;
  return editingTask.state !== 'TODO' && editingTask.state !== 'CREATED';
}

type DialogComputedValues = {
  isPassthroughProfile: boolean;
  effectiveWorkflowId: string | null;
  effectiveDefaultStepId: string | null;
  workspaceDefaults: Workspace | null | undefined;
  hasRepositorySelection: boolean;
  branchOptions: ReturnType<typeof useBranchOptions>;
  agentProfileOptions: ReturnType<typeof useAgentProfileOptions>;
  executorOptions: ReturnType<typeof useExecutorOptions>;
  executorHint: string | null;
  headerRepositoryOptions: ReturnType<typeof useRepositoryOptions>['headerRepositoryOptions'];
  agentProfilesLoading: boolean;
  executorsLoading: boolean;
};

type DialogComputedArgs = {
  fs: DialogFormState;
  open: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  defaultStepId: string | null;
  branches: Branch[];
  settingsData: { agentsLoaded: boolean; executorsLoaded: boolean };
  agentProfiles: AgentProfileOption[];
  workspaces: Workspace[];
  executors: Executor[];
  repositories: Repository[];
};

function useDialogComputed({ fs, open, workspaceId, workflowId, defaultStepId, branches, settingsData, agentProfiles, workspaces, executors, repositories }: DialogComputedArgs): DialogComputedValues {
  const isPassthroughProfile = useMemo(() => computePassthroughProfile(fs.agentProfileId, agentProfiles), [fs.agentProfileId, agentProfiles]);
  const effectiveWorkflowId = fs.selectedWorkflowId ?? workflowId;
  const effectiveDefaultStepId = computeEffectiveStepId(fs.selectedWorkflowId, workflowId, fs.fetchedSteps, defaultStepId);
  const workspaceDefaults = workspaceId ? workspaces.find((ws: Workspace) => ws.id === workspaceId) : null;
  const hasRepositorySelection = Boolean(fs.repositoryId || fs.selectedLocalRepo);
  const branchOptions = useBranchOptions(fs.repositoryId ? branches : fs.localBranches);
  const agentProfileOptions = useAgentProfileOptions(agentProfiles);
  const executorOptions = useExecutorOptions(executors);
  const executorHint = useExecutorHint(executors, fs.executorId);
  const { headerRepositoryOptions } = useRepositoryOptions(repositories, fs.discoveredRepositories);
  const agentProfilesLoading = open && !settingsData.agentsLoaded;
  const executorsLoading = open && !settingsData.executorsLoaded;
  return { isPassthroughProfile, effectiveWorkflowId, effectiveDefaultStepId, workspaceDefaults, hasRepositorySelection, branchOptions, agentProfileOptions, executorOptions, executorHint, headerRepositoryOptions, agentProfilesLoading, executorsLoading };
}

function useSessionRepoName(isSessionMode: boolean) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);
  const reposByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  return useMemo(() => {
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
}

export function TaskCreateDialog({
  open, onOpenChange, mode = 'create', workspaceId, workflowId, defaultStepId,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  steps, editingTask, onSuccess, onCreateSession, initialValues, taskId = null,
}: TaskCreateDialogProps) {
  const isSessionMode = mode === 'session';
  const isEditMode = mode === 'edit';
  const isCreateMode = mode === 'create';
  const isTaskStarted = computeIsTaskStarted(isEditMode, editingTask);

  const fs = useDialogFormState(open, workspaceId, workflowId, initialValues);

  const workflows = useAppStore((state) => state.workflows.items);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const environments = useAppStore((state) => state.environments.items);
  const settingsData = useAppStore((state) => state.settingsData);
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const sessionRepoName = useSessionRepoName(isSessionMode);

  const { toast } = useToast();
  useSettingsData(open);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, open);
  const { branches, isLoading: branchesLoading } = useRepositoryBranches(fs.repositoryId || null, Boolean(open && fs.repositoryId));

  const computed = useDialogComputed({ fs, open, workspaceId, workflowId, defaultStepId, branches, settingsData, agentProfiles, workspaces, executors, repositories });
  const { isPassthroughProfile, effectiveWorkflowId, effectiveDefaultStepId, workspaceDefaults, hasRepositorySelection, branchOptions, agentProfileOptions, executorOptions, executorHint, headerRepositoryOptions, agentProfilesLoading, executorsLoading } = computed;

  useWorkflowStepsEffect(fs, workflowId);
  useRepositoryAutoSelectEffect(fs, open, workspaceId, repositories);
  useDiscoverReposEffect(fs, open, workspaceId, repositoriesLoading, toast);
  useBranchAutoSelectEffect(fs, branches);
  useLocalBranchesEffect(fs, open, workspaceId, toast);
  useDefaultSelectionsEffect(fs, open, { agentProfiles, environments, executors, workspaceDefaults });

  const { handleRepositoryChange, handleAgentProfileChange, handleTaskNameChange, handleBranchChange, handleWorkflowChange } = useDialogHandlers(fs, repositories);

  const { handleSubmit, handleUpdateWithoutAgent, handleCreateWithoutAgent, handleCancel } = useTaskSubmitHandlers({
    isSessionMode, isEditMode, isPassthroughProfile,
    taskName: fs.taskName, workspaceId, workflowId, effectiveWorkflowId, effectiveDefaultStepId,
    repositoryId: fs.repositoryId, selectedLocalRepo: fs.selectedLocalRepo, branch: fs.branch,
    agentProfileId: fs.agentProfileId, environmentId: fs.environmentId, executorId: fs.executorId,
    editingTask, onSuccess, onCreateSession, onOpenChange, taskId,
    descriptionInputRef: fs.descriptionInputRef, setIsCreatingSession: fs.setIsCreatingSession,
    setIsCreatingTask: fs.setIsCreatingTask, setHasTitle: fs.setHasTitle, setHasDescription: fs.setHasDescription,
    setTaskName: fs.setTaskName, setRepositoryId: fs.setRepositoryId, setBranch: fs.setBranch,
    setAgentProfileId: fs.setAgentProfileId, setEnvironmentId: fs.setEnvironmentId,
    setExecutorId: fs.setExecutorId, setSelectedWorkflowId: fs.setSelectedWorkflowId, setFetchedSteps: fs.setFetchedSteps,
  });

  const handleKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, (event) => {
    handleSubmit(event as unknown as FormEvent);
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full h-full max-w-full max-h-full rounded-none sm:w-[900px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col">
        <DialogHeader>
          <DialogHeaderContent
            isCreateMode={isCreateMode} isEditMode={isEditMode} isTaskStarted={isTaskStarted}
            sessionRepoName={sessionRepoName} initialTitle={initialValues?.title}
            taskName={fs.taskName} repositoryId={fs.repositoryId} discoveredRepoPath={fs.discoveredRepoPath}
            workspaceId={workspaceId} repositoriesLoading={repositoriesLoading} discoverReposLoading={fs.discoverReposLoading}
            headerRepositoryOptions={headerRepositoryOptions} onRepositoryChange={handleRepositoryChange} onTaskNameChange={handleTaskNameChange}
          />
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4 overflow-hidden">
          <DialogFormBody
            open={open} isSessionMode={isSessionMode} isCreateMode={isCreateMode}
            isTaskStarted={isTaskStarted} isPassthroughProfile={isPassthroughProfile}
            initialDescription={initialValues?.description ?? ''} hasDescription={fs.hasDescription}
            branchOptions={branchOptions} branchesLoading={branchesLoading}
            agentProfileOptions={agentProfileOptions} executorOptions={executorOptions}
            agentProfiles={agentProfiles} agentProfilesLoading={agentProfilesLoading}
            executorsLoading={executorsLoading} isCreatingSession={fs.isCreatingSession}
            workflows={workflows} snapshots={snapshots} effectiveWorkflowId={effectiveWorkflowId ?? null}
            fs={fs} handleKeyDown={handleKeyDown} onBranchChange={handleBranchChange}
            onAgentProfileChange={handleAgentProfileChange} onWorkflowChange={handleWorkflowChange}
            hasRepositorySelection={hasRepositorySelection}
          />
          <DialogFooter className="border-t border-border pt-3 flex-col gap-3 sm:flex-row sm:gap-2">
            <TaskCreateDialogFooter
              isSessionMode={isSessionMode} isCreateMode={isCreateMode} isEditMode={isEditMode}
              isTaskStarted={isTaskStarted} isPassthroughProfile={isPassthroughProfile}
              isCreatingSession={fs.isCreatingSession} isCreatingTask={fs.isCreatingTask}
              hasTitle={fs.hasTitle} hasDescription={fs.hasDescription} hasRepositorySelection={hasRepositorySelection}
              branch={fs.branch} agentProfileId={fs.agentProfileId} workspaceId={workspaceId}
              effectiveWorkflowId={effectiveWorkflowId ?? null} executorHint={executorHint}
              onCancel={handleCancel} onUpdateWithoutAgent={handleUpdateWithoutAgent}
              onCreateWithoutAgent={handleCreateWithoutAgent}
            />
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
