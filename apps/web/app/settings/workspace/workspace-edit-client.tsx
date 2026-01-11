'use client';

import { useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { IconGitBranch, IconPlus, IconTrash } from '@tabler/icons-react';
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
  deleteBoardAction,
  deleteColumnAction,
  deleteWorkspaceAction,
  updateBoardAction,
  updateColumnAction,
  updateWorkspaceAction,
} from '@/app/actions/workspaces';
import type { Board, Column, Workspace } from '@/lib/types/http';
import type { Repository } from '@/lib/settings/types';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';

type BoardWithColumns = Board & { columns: Column[] };

type WorkspaceEditClientProps = {
  workspace: Workspace | null;
  boards: BoardWithColumns[];
};

export function WorkspaceEditClient({ workspace, boards }: WorkspaceEditClientProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace | null>(workspace);
  const [workspaceNameDraft, setWorkspaceNameDraft] = useState(workspace?.name ?? '');
  const [boardItems, setBoardItems] = useState<BoardWithColumns[]>(boards);
  const [savedWorkspaceName, setSavedWorkspaceName] = useState(workspace?.name ?? '');
  const [savedBoardItems, setSavedBoardItems] = useState<BoardWithColumns[]>(boards);
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const saveWorkspaceRequest = useRequest(updateWorkspaceAction);
  const deleteWorkspaceRequest = useRequest(deleteWorkspaceAction);

  const handleSaveWorkspaceName = async () => {
    if (!currentWorkspace) return;
    const trimmed = workspaceNameDraft.trim();
    if (!trimmed || trimmed === currentWorkspace.name) return;
    try {
      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, { name: trimmed });
      setCurrentWorkspace((prev) => (prev ? { ...prev, ...updated } : prev));
      setSavedWorkspaceName(updated.name);
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

  const handleAddRepository = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    const newRepo: Repository = {
      id: crypto.randomUUID(),
      name: formData.get('name') as string,
      path: formData.get('path') as string,
      setupScript: formData.get('setupScript') as string,
      cleanupScript: formData.get('cleanupScript') as string,
      customScripts: [],
    };
    setRepositories((prev) => [...prev, newRepo]);
  };

  const handleUpdateRepository = (repo: Repository) => {
    setRepositories((prev) => prev.map((item) => (item.id === repo.id ? repo : item)));
  };

  const handleDeleteRepository = (id: string) => {
    setRepositories((prev) => prev.filter((repo) => repo.id !== id));
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
      >
        <div className="grid gap-3">
          <Card>
            <CardContent className="pt-6">
              <form onSubmit={handleAddRepository} className="space-y-4">
                <div className="space-y-2">
                  <Label>Repository Name</Label>
                  <Input name="name" placeholder="my-project" required />
                </div>
                <div className="space-y-2">
                  <Label>Directory Path</Label>
                  <Input name="path" placeholder="/path/to/repository" required />
                </div>
                <div className="space-y-2">
                  <Label>Setup Script</Label>
                  <Input name="setupScript" placeholder="npm install" />
                </div>
                <div className="space-y-2">
                  <Label>Cleanup Script</Label>
                  <Input name="cleanupScript" placeholder="npm run cleanup" />
                </div>
                <div className="flex justify-end">
                  <Button type="submit">Add Repository</Button>
                </div>
              </form>
            </CardContent>
          </Card>

          {repositories.map((repo) => (
            <RepositoryCard
              key={repo.id}
              repository={repo}
              onUpdate={handleUpdateRepository}
              onDelete={() => handleDeleteRepository(repo.id)}
            />
          ))}
        </div>
      </SettingsSection>

      <Separator />

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
