'use client';

import { useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconGitBranch, IconLoader2, IconPlus, IconTrash } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SettingsSection } from '@/components/settings/settings-section';
import { RepositoryCard } from '@/components/settings/repository-card';
import { BoardCard } from '@/components/settings/board-card';
import {
  createBoardAction,
  createColumnAction,
  discoverRepositoriesAction,
  createRepositoryAction,
  createRepositoryScriptAction,
  deleteBoardAction,
  deleteColumnAction,
  deleteRepositoryAction,
  deleteRepositoryScriptAction,
  deleteWorkspaceAction,
  updateBoardAction,
  updateColumnAction,
  validateRepositoryPathAction,
  updateRepositoryAction,
  updateRepositoryScriptAction,
  updateWorkspaceAction,
} from '@/app/actions/workspaces';
import type {
  Board,
  Column,
  LocalRepository,
  Repository,
  RepositoryScript,
  Workspace,
} from '@/lib/types/http';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { useAppStore } from '@/components/state-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';

type BoardWithColumns = Board & { columns: Column[] };
type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

type WorkspaceEditClientProps = {
  workspace: Workspace | null;
  boards: BoardWithColumns[];
  repositories: RepositoryWithScripts[];
};

export function WorkspaceEditClient({ workspace, boards, repositories }: WorkspaceEditClientProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace | null>(workspace);
  const [workspaceNameDraft, setWorkspaceNameDraft] = useState(workspace?.name ?? '');
  const [boardItems, setBoardItems] = useState<BoardWithColumns[]>(boards);
  const [savedWorkspaceName, setSavedWorkspaceName] = useState(workspace?.name ?? '');
  const [savedBoardItems, setSavedBoardItems] = useState<BoardWithColumns[]>(boards);
  const [repositoryItems, setRepositoryItems] = useState<RepositoryWithScripts[]>(repositories);
  const [savedRepositoryItems, setSavedRepositoryItems] = useState<RepositoryWithScripts[]>(repositories);
  const [discoveredRepositories, setDiscoveredRepositories] = useState<LocalRepository[]>([]);
  const [localRepoDialogOpen, setLocalRepoDialogOpen] = useState(false);
  const [remoteRepoDialogOpen, setRemoteRepoDialogOpen] = useState(false);
  const [repoSearch, setRepoSearch] = useState('');
  const [selectedRepoPath, setSelectedRepoPath] = useState<string | null>(null);
  const [manualRepoPath, setManualRepoPath] = useState('');
  const [manualValidation, setManualValidation] = useState<{
    status: 'idle' | 'loading' | 'success' | 'error';
    message?: string;
    isValid?: boolean;
    path?: string;
  }>({ status: 'idle' });
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const saveWorkspaceRequest = useRequest(updateWorkspaceAction);
  const deleteWorkspaceRequest = useRequest(deleteWorkspaceAction);
  const discoverRepositoriesRequest = useRequest(discoverRepositoriesAction);
  const validateRepositoryPathRequest = useRequest(validateRepositoryPathAction);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);

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
          workspace.id === updated.id ? { ...workspace, name: updated.name } : workspace
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

  const cloneBoard = (board: BoardWithColumns): BoardWithColumns => ({
    ...board,
    columns: board.columns.map((column) => ({ ...column })),
  });

  const savedBoardsById = useMemo(() => {
    return new Map(savedBoardItems.map((board) => [board.id, board]));
  }, [savedBoardItems]);

  const isBoardNameDirty = (board: BoardWithColumns) => {
    const saved = savedBoardsById.get(board.id);
    if (!saved) return true;
    if (board.name !== saved.name || board.description !== saved.description) return true;
    return false;
  };

  const areColumnsDirty = (board: BoardWithColumns) => {
    const saved = savedBoardsById.get(board.id);
    if (!saved) {
      return board.columns.length > 0;
    }
    if (board.columns.length !== saved.columns.length) return true;
    const savedColumns = new Map(saved.columns.map((column) => [column.id, column]));
    for (const column of board.columns) {
      const savedColumn = savedColumns.get(column.id);
      if (!savedColumn) return true;
      if (
        column.name !== savedColumn.name ||
        column.color !== savedColumn.color ||
        column.position !== savedColumn.position ||
        column.state !== savedColumn.state
      ) {
        return true;
      }
    }
    return false;
  };

  const isWorkspaceDirty = workspaceNameDraft.trim() !== savedWorkspaceName;

  const cloneRepository = (repo: RepositoryWithScripts): RepositoryWithScripts => ({
    ...repo,
    scripts: repo.scripts.map((script) => ({ ...script })),
  });

  const savedRepositoriesById = useMemo(() => {
    return new Map(savedRepositoryItems.map((repo) => [repo.id, repo]));
  }, [savedRepositoryItems]);

  const filteredRepositories = useMemo(() => {
    const query = repoSearch.trim().toLowerCase();
    if (!query) return discoveredRepositories;
    return discoveredRepositories.filter(
      (repo) => repo.name.toLowerCase().includes(query) || repo.path.toLowerCase().includes(query)
    );
  }, [discoveredRepositories, repoSearch]);

  const canSaveLocalRepo =
    Boolean(selectedRepoPath) || (manualValidation.status === 'success' && manualValidation.isValid);

  const isRepositoryDirty = (repo: RepositoryWithScripts) => {
    const saved = savedRepositoriesById.get(repo.id);
    if (!saved) return true;
    return (
      repo.name !== saved.name ||
      repo.source_type !== saved.source_type ||
      repo.local_path !== saved.local_path ||
      repo.provider !== saved.provider ||
      repo.provider_repo_id !== saved.provider_repo_id ||
      repo.provider_owner !== saved.provider_owner ||
      repo.provider_name !== saved.provider_name ||
      repo.default_branch !== saved.default_branch ||
      repo.setup_script !== saved.setup_script ||
      repo.cleanup_script !== saved.cleanup_script
    );
  };

  const areRepositoryScriptsDirty = (repo: RepositoryWithScripts) => {
    const saved = savedRepositoriesById.get(repo.id);
    if (!saved) return repo.scripts.length > 0;
    if (repo.scripts.length !== saved.scripts.length) return true;
    const savedScripts = new Map(saved.scripts.map((script) => [script.id, script]));
    for (const script of repo.scripts) {
      const savedScript = savedScripts.get(script.id);
      if (!savedScript) return true;
      if (
        script.name !== savedScript.name ||
        script.command !== savedScript.command ||
        script.position !== savedScript.position
      ) {
        return true;
      }
    }
    return false;
  };

  const handleUpdateRepository = (repoId: string, updates: Partial<Repository>) => {
    setRepositoryItems((prev) =>
      prev.map((repo) => (repo.id === repoId ? { ...repo, ...updates } : repo))
    );
  };

  const handleDiscoverRepositories = async () => {
    if (!currentWorkspace) return;
    try {
      const result = await discoverRepositoriesRequest.run(currentWorkspace.id);
      setDiscoveredRepositories(result.repositories);
    } catch (error) {
      toast({
        title: 'Failed to discover repositories',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const openLocalRepositoryDialog = async () => {
    setLocalRepoDialogOpen(true);
    setRepoSearch('');
    setSelectedRepoPath(null);
    setManualRepoPath('');
    setManualValidation({ status: 'idle' });
    await handleDiscoverRepositories();
  };

  const handleValidateManualPath = async () => {
    if (!currentWorkspace || !manualRepoPath.trim()) return;
    setManualValidation({ status: 'loading' });
    try {
      const result = await validateRepositoryPathRequest.run(currentWorkspace.id, manualRepoPath.trim());
      if (result.allowed && result.exists && result.is_git) {
        setManualValidation({
          status: 'success',
          isValid: true,
          message: 'Valid git repository',
          path: result.path,
        });
      } else {
        setManualValidation({
          status: 'error',
          isValid: false,
          message: result.message || 'Invalid repository path',
          path: result.path,
        });
      }
    } catch (error) {
      setManualValidation({
        status: 'error',
        isValid: false,
        message: error instanceof Error ? error.message : 'Request failed',
      });
    }
  };

  const handleConfirmLocalRepository = () => {
    if (!currentWorkspace) return;
    const selectedRepo = discoveredRepositories.find((repo) => repo.path === selectedRepoPath);
    const path = selectedRepo?.path || manualValidation.path || manualRepoPath.trim();
    if (!path) return;
    const name = selectedRepo?.name || path.split('/').filter(Boolean).slice(-1)[0] || 'New Repository';
    const draftId = `temp-repo-${crypto.randomUUID()}`;
    const draftRepo: RepositoryWithScripts = {
      id: draftId,
      workspace_id: currentWorkspace.id,
      name,
      source_type: 'local',
      local_path: path,
      provider: '',
      provider_repo_id: '',
      provider_owner: '',
      provider_name: '',
      default_branch: selectedRepo?.default_branch || '',
      setup_script: '',
      cleanup_script: '',
      created_at: '',
      updated_at: '',
      scripts: [],
    };
    setRepositoryItems((prev) => [draftRepo, ...prev]);
    setLocalRepoDialogOpen(false);
  };

  const handleAddRepositoryScript = (repoId: string) => {
    const script: RepositoryScript = {
      id: `temp-script-${crypto.randomUUID()}`,
      repository_id: repoId,
      name: '',
      command: '',
      position: repositoryItems.find((repo) => repo.id === repoId)?.scripts.length ?? 0,
      created_at: '',
      updated_at: '',
    };
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId ? { ...repo, scripts: [...repo.scripts, script] } : repo
      )
    );
  };

  const handleUpdateRepositoryScript = (repoId: string, scriptId: string, updates: Partial<RepositoryScript>) => {
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId
          ? {
              ...repo,
              scripts: repo.scripts.map((script) =>
                script.id === scriptId ? { ...script, ...updates } : script
              ),
            }
          : repo
      )
    );
  };

  const handleDeleteRepositoryScript = (repoId: string, scriptId: string) => {
    setRepositoryItems((prev) =>
      prev.map((repo) =>
        repo.id === repoId
          ? { ...repo, scripts: repo.scripts.filter((script) => script.id !== scriptId) }
          : repo
      )
    );
  };

  const handleSaveRepository = async (repoId: string) => {
    const repo = repositoryItems.find((item) => item.id === repoId);
    if (!repo) return;
    if (repoId.startsWith('temp-repo-')) {
      const created = await createRepositoryAction({
        workspace_id: currentWorkspace?.id ?? repo.workspace_id,
        name: repo.name.trim() || 'New Repository',
        source_type: repo.source_type || 'local',
        local_path: repo.local_path,
        provider: repo.provider,
        provider_repo_id: repo.provider_repo_id,
        provider_owner: repo.provider_owner,
        provider_name: repo.provider_name,
        default_branch: repo.default_branch,
        setup_script: repo.setup_script,
        cleanup_script: repo.cleanup_script,
      });
      const scripts = await Promise.all(
        repo.scripts.map((script, index) =>
          createRepositoryScriptAction({
            repository_id: created.id,
            name: script.name.trim() || 'New Script',
            command: script.command.trim() || 'echo ""',
            position: script.position ?? index,
          })
        )
      );
      const nextRepo: RepositoryWithScripts = { ...created, scripts };
      setRepositoryItems((prev) => prev.map((item) => (item.id === repoId ? nextRepo : item)));
      setSavedRepositoryItems((prev) => [cloneRepository(nextRepo), ...prev]);
      return;
    }

    const updated = await updateRepositoryAction(repoId, {
      name: repo.name,
      source_type: repo.source_type,
      local_path: repo.local_path,
      provider: repo.provider,
      provider_repo_id: repo.provider_repo_id,
      provider_owner: repo.provider_owner,
      provider_name: repo.provider_name,
      default_branch: repo.default_branch,
      setup_script: repo.setup_script,
      cleanup_script: repo.cleanup_script,
    });

    const saved = savedRepositoriesById.get(repoId);
    const savedScripts = saved ? saved.scripts : [];
    const currentScriptIds = new Set(repo.scripts.map((script) => script.id));

    await Promise.all(
      savedScripts
        .filter((script) => !currentScriptIds.has(script.id))
        .map((script) => deleteRepositoryScriptAction(script.id))
    );

    const nextScripts = await Promise.all(
      repo.scripts.map((script, index) => {
        if (script.id.startsWith('temp-script-')) {
          return createRepositoryScriptAction({
            repository_id: repoId,
            name: script.name.trim() || 'New Script',
            command: script.command.trim() || 'echo ""',
            position: script.position ?? index,
          });
        }
        return updateRepositoryScriptAction(script.id, {
          name: script.name,
          command: script.command,
          position: script.position ?? index,
        });
      })
    );

    const nextRepo: RepositoryWithScripts = { ...updated, scripts: nextScripts };
    setRepositoryItems((prev) => prev.map((item) => (item.id === repoId ? nextRepo : item)));
    setSavedRepositoryItems((prev) =>
      prev.some((item) => item.id === repoId)
        ? prev.map((item) => (item.id === repoId ? cloneRepository(nextRepo) : item))
        : [...prev, cloneRepository(nextRepo)]
    );
  };

  const handleDeleteRepository = async (repoId: string) => {
    if (repoId.startsWith('temp-repo-')) {
      setRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
      return;
    }
    await deleteRepositoryAction(repoId);
    setRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
    setSavedRepositoryItems((prev) => prev.filter((repo) => repo.id !== repoId));
  };

  const handleAddBoard = () => {
    if (!currentWorkspace) return;
    const draftBoardId = `temp-${crypto.randomUUID()}`;
    const draftBoard: BoardWithColumns = {
      id: draftBoardId,
      workspace_id: currentWorkspace.id,
      name: 'New Board',
      description: '',
      created_at: '',
      updated_at: '',
      columns: [
        {
          id: `temp-col-${crypto.randomUUID()}`,
          board_id: draftBoardId,
          name: 'Todo',
          position: 0,
          state: 'TODO',
          color: 'bg-cyan-500',
          created_at: '',
          updated_at: '',
        },
        {
          id: `temp-col-${crypto.randomUUID()}`,
          board_id: draftBoardId,
          name: 'In Progress',
          position: 1,
          state: 'IN_PROGRESS',
          color: 'bg-yellow-500',
          created_at: '',
          updated_at: '',
        },
        {
          id: `temp-col-${crypto.randomUUID()}`,
          board_id: draftBoardId,
          name: 'To Review',
          position: 2,
          state: 'REVIEW',
          color: 'bg-green-500',
          created_at: '',
          updated_at: '',
        },
        {
          id: `temp-col-${crypto.randomUUID()}`,
          board_id: draftBoardId,
          name: 'Done',
          position: 3,
          state: 'COMPLETED',
          color: 'bg-indigo-500',
          created_at: '',
          updated_at: '',
        },
      ],
    };
    setBoardItems((prev) => [draftBoard, ...prev]);
  };

  const handleUpdateBoard = (boardId: string, updates: { name?: string; description?: string }) => {
    setBoardItems((prev) =>
      prev.map((board) => (board.id === boardId ? { ...board, ...updates } : board))
    );
  };

  const handleDeleteBoard = async (boardId: string) => {
    if (boardId.startsWith('temp-')) {
      setBoardItems((prev) => prev.filter((board) => board.id !== boardId));
      return;
    }
    await deleteBoardAction(boardId);
    setBoardItems((prev) => prev.filter((board) => board.id !== boardId));
    setSavedBoardItems((prev) => prev.filter((board) => board.id !== boardId));
  };

  const handleCreateColumn = (
    boardId: string,
    column: Omit<Column, 'id' | 'board_id' | 'created_at' | 'updated_at'>
  ) => {
    const draftColumn: Column = {
      id: `temp-col-${crypto.randomUUID()}`,
      board_id: boardId,
      name: column.name,
      position: column.position,
      state: column.state,
      color: column.color,
      created_at: '',
      updated_at: '',
    };
    setBoardItems((prev) =>
      prev.map((board) =>
        board.id === boardId ? { ...board, columns: [...board.columns, draftColumn] } : board
      )
    );
  };

  const handleUpdateColumn = (boardId: string, columnId: string, updates: Partial<Column>) => {
    setBoardItems((prev) =>
      prev.map((board) =>
        board.id === boardId
          ? {
              ...board,
              columns: board.columns.map((column) => (column.id === columnId ? { ...column, ...updates } : column)),
            }
          : board
      )
    );
  };

  const handleDeleteColumn = async (boardId: string, columnId: string) => {
    if (boardId.startsWith('temp-')) {
      setBoardItems((prev) =>
        prev.map((board) =>
          board.id === boardId ? { ...board, columns: board.columns.filter((column) => column.id !== columnId) } : board
        )
      );
      return;
    }
    await deleteColumnAction(columnId);
    setBoardItems((prev) =>
      prev.map((board) =>
        board.id === boardId ? { ...board, columns: board.columns.filter((column) => column.id !== columnId) } : board
      )
    );
  };

  const handleReorderColumns = (boardId: string, columns: Column[]) => {
    setBoardItems((prev) =>
      prev.map((board) => (board.id === boardId ? { ...board, columns } : board))
    );
  };

  const handleSaveBoard = async (boardId: string) => {
    const board = boardItems.find((item) => item.id === boardId);
    if (!board) return;
    if (boardId.startsWith('temp-')) {
      const name = board.name.trim() || 'New Board';
      const createdBoard = await createBoardAction({
        workspace_id: currentWorkspace?.id ?? board.workspace_id,
        name,
      });
      const createdColumns = await Promise.all(
        board.columns.map((column, index) =>
          createColumnAction({
            board_id: createdBoard.id,
            name: column.name.trim() || 'New Column',
            position: column.position ?? index,
            state: column.state,
            color: column.color,
          })
        )
      );
      const nextBoard = {
        ...createdBoard,
        columns: createdColumns,
      };
      setBoardItems((prev) =>
        prev.map((item) => (item.id === boardId ? nextBoard : item))
      );
      setSavedBoardItems((prev) => [cloneBoard(nextBoard), ...prev]);
      return;
    }
    const updates: { name?: string; description?: string } = {};
    if (board.name.trim()) {
      updates.name = board.name.trim();
    }
    if (Object.keys(updates).length) {
      await updateBoardAction(boardId, updates);
    }
    const nextColumns = await Promise.all(
      board.columns.map((column, index) => {
        if (column.id.startsWith('temp-col-')) {
          return createColumnAction({
            board_id: boardId,
            name: column.name.trim() || 'New Column',
            position: column.position ?? index,
            state: column.state,
            color: column.color,
          });
        }
        return updateColumnAction(column.id, {
          name: column.name,
          color: column.color,
          position: column.position ?? index,
          state: column.state,
        });
      })
    );
    setBoardItems((prev) =>
      prev.map((item) => (item.id === boardId ? { ...item, columns: nextColumns } : item))
    );
    setSavedBoardItems((prev) =>
      prev.some((item) => item.id === boardId)
        ? prev.map((item) =>
            item.id === boardId ? cloneBoard({ ...board, columns: nextColumns }) : item
          )
        : [...prev, cloneBoard({ ...board, columns: nextColumns })]
    );
  };

  const handleDeleteWorkspace = async () => {
    if (deleteConfirmText !== 'delete') return;
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
          Manage repositories and boards for this workspace
        </p>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span>Workspace Name</span>
            {isWorkspaceDirty && <UnsavedChangesBadge />}
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
                isDirty={isWorkspaceDirty}
                isLoading={saveWorkspaceRequest.isLoading}
                status={saveWorkspaceRequest.status}
                onClick={handleSaveWorkspaceName}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <Separator />

      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title="Repositories"
        description="Repositories in this workspace"
        action={
          <div className="flex items-center gap-2">
            <Button size="sm" onClick={openLocalRepositoryDialog}>
              Local Repository
            </Button>
            <Button size="sm" variant="outline" onClick={() => setRemoteRepoDialogOpen(true)}>
              Remote Repository
            </Button>
          </div>
        }
      >
        <div className="grid gap-3">
          {repositoryItems.map((repo) => (
            <RepositoryCard
              key={repo.id}
              repository={repo}
              isRepositoryDirty={isRepositoryDirty(repo)}
              areScriptsDirty={areRepositoryScriptsDirty(repo)}
              onUpdate={handleUpdateRepository}
              onAddScript={handleAddRepositoryScript}
              onUpdateScript={handleUpdateRepositoryScript}
              onDeleteScript={handleDeleteRepositoryScript}
              onSave={handleSaveRepository}
              onDelete={handleDeleteRepository}
            />
          ))}
        </div>
      </SettingsSection>

      <Separator />

      <Dialog open={localRepoDialogOpen} onOpenChange={setLocalRepoDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Add Local Repository</DialogTitle>
            <DialogDescription>
              Select a discovered repository or provide an absolute path to validate.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Discovered repositories</Label>
              <Input
                placeholder="Filter repositories..."
                value={repoSearch}
                onChange={(e) => setRepoSearch(e.target.value)}
              />
              <div className="max-h-56 overflow-auto rounded-md border border-border">
                {discoverRepositoriesRequest.isLoading ? (
                  <div className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
                    <IconLoader2 className="h-4 w-4 animate-spin" />
                    Scanning repositories...
                  </div>
                ) : filteredRepositories.length === 0 ? (
                  <div className="p-3 text-sm text-muted-foreground">No repositories found.</div>
                ) : (
                  filteredRepositories.map((repo) => (
                    <button
                      key={repo.path}
                      type="button"
                      className={`flex w-full flex-col px-3 py-2 text-left text-sm hover:bg-muted ${
                        selectedRepoPath === repo.path ? 'bg-muted' : ''
                      }`}
                      onClick={() => {
                        setSelectedRepoPath(repo.path);
                        setManualRepoPath('');
                        setManualValidation({ status: 'idle' });
                      }}
                    >
                      <span className="font-medium">{repo.name}</span>
                      <span className="text-xs text-muted-foreground">{repo.path}</span>
                    </button>
                  ))
                )}
              </div>
            </div>

            <div className="space-y-2">
              <Label>Manual path</Label>
              <div className="flex items-center gap-2">
                <Input
                  placeholder="/absolute/path/to/repository"
                  value={manualRepoPath}
                  onChange={(e) => {
                    setManualRepoPath(e.target.value);
                    setSelectedRepoPath(null);
                    setManualValidation({ status: 'idle' });
                  }}
                />
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleValidateManualPath}
                  disabled={!manualRepoPath.trim() || validateRepositoryPathRequest.isLoading}
                >
                  {validateRepositoryPathRequest.isLoading ? 'Checking...' : 'Validate'}
                </Button>
              </div>
              {manualValidation.status === 'error' && (
                <p className="text-xs text-destructive">{manualValidation.message}</p>
              )}
              {manualValidation.status === 'success' && (
                <p className="text-xs text-emerald-500">{manualValidation.message}</p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setLocalRepoDialogOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={handleConfirmLocalRepository} disabled={!canSaveLocalRepo}>
              Use Repository
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={remoteRepoDialogOpen} onOpenChange={setRemoteRepoDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Remote Repository</DialogTitle>
            <DialogDescription>Remote providers are coming soon.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Button type="button" variant="outline" className="w-full justify-start" disabled>
              GitHub (coming soon)
            </Button>
            <Button type="button" variant="outline" className="w-full justify-start" disabled>
              GitLab (coming soon)
            </Button>
            <Button type="button" variant="outline" className="w-full justify-start" disabled>
              Bitbucket (coming soon)
            </Button>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setRemoteRepoDialogOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title="Boards"
        description="Boards and columns in this workspace"
        action={
          <Button size="sm" onClick={handleAddBoard}>
            <IconPlus className="h-4 w-4 mr-2" />
            Add Board
          </Button>
        }
      >
        <div className="grid gap-3">
          {boardItems.map((board) => (
            <BoardCard
              key={board.id}
              board={board}
              columns={board.columns}
              isBoardDirty={isBoardNameDirty(board)}
              areColumnsDirty={areColumnsDirty(board)}
              onUpdateBoard={(updates) => handleUpdateBoard(board.id, updates)}
              onDeleteBoard={() => handleDeleteBoard(board.id)}
              onCreateColumn={(column) => handleCreateColumn(board.id, column)}
              onUpdateColumn={(columnId, updates) => handleUpdateColumn(board.id, columnId, updates)}
              onDeleteColumn={(columnId) => handleDeleteColumn(board.id, columnId)}
              onReorderColumns={(columns) => handleReorderColumns(board.id, columns)}
              onSaveBoard={() => handleSaveBoard(board.id)}
            />
          ))}
        </div>
      </SettingsSection>

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
