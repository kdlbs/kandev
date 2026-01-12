'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { IconLayoutColumns, IconPlus } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { SettingsSection } from '@/components/settings/settings-section';
import { BoardCard } from '@/components/settings/board-card';
import { generateUUID } from '@/lib/utils';
import {
  createBoardAction,
  createColumnAction,
  deleteBoardAction,
  deleteColumnAction,
  updateBoardAction,
  updateColumnAction,
} from '@/app/actions/workspaces';
import type { Board, Column, Workspace } from '@/lib/types/http';

type BoardWithColumns = Board & { columns: Column[] };

type WorkspaceBoardsClientProps = {
  workspace: Workspace | null;
  boards: BoardWithColumns[];
};

export function WorkspaceBoardsClient({ workspace, boards }: WorkspaceBoardsClientProps) {
  const router = useRouter();
  const [boardItems, setBoardItems] = useState<BoardWithColumns[]>(boards);
  const [savedBoardItems, setSavedBoardItems] = useState<BoardWithColumns[]>(boards);

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

  const handleAddBoard = () => {
    if (!workspace) return;
    const draftBoardId = `temp-${generateUUID()}`;
    const draftBoard: BoardWithColumns = {
      id: draftBoardId,
      workspace_id: workspace.id,
      name: 'New Board',
      description: '',
      created_at: '',
      updated_at: '',
      columns: [
        {
          id: `temp-col-${generateUUID()}`,
          board_id: draftBoardId,
          name: 'Todo',
          position: 0,
          state: 'TODO',
          color: 'bg-cyan-500',
          created_at: '',
          updated_at: '',
        },
        {
          id: `temp-col-${generateUUID()}`,
          board_id: draftBoardId,
          name: 'In Progress',
          position: 1,
          state: 'IN_PROGRESS',
          color: 'bg-yellow-500',
          created_at: '',
          updated_at: '',
        },
        {
          id: `temp-col-${generateUUID()}`,
          board_id: draftBoardId,
          name: 'To Review',
          position: 2,
          state: 'REVIEW',
          color: 'bg-green-500',
          created_at: '',
          updated_at: '',
        },
        {
          id: `temp-col-${generateUUID()}`,
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
      id: `temp-col-${generateUUID()}`,
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
              columns: board.columns.map((column) =>
                column.id === columnId ? { ...column, ...updates } : column
              ),
            }
          : board
      )
    );
  };

  const handleDeleteColumn = async (boardId: string, columnId: string) => {
    if (boardId.startsWith('temp-')) {
      setBoardItems((prev) =>
        prev.map((board) =>
          board.id === boardId
            ? { ...board, columns: board.columns.filter((column) => column.id !== columnId) }
            : board
        )
      );
      return;
    }
    await deleteColumnAction(columnId);
    setBoardItems((prev) =>
      prev.map((board) =>
        board.id === boardId
          ? { ...board, columns: board.columns.filter((column) => column.id !== columnId) }
          : board
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
        workspace_id: workspace?.id ?? board.workspace_id,
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
      setBoardItems((prev) => prev.map((item) => (item.id === boardId ? nextBoard : item)));
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

  if (!workspace) {
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
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">{workspace.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Manage boards and columns for this workspace.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`/settings/workspace/${workspace.id}`}>Workspace settings</Link>
        </Button>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconLayoutColumns className="h-5 w-5" />}
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
              onUpdateColumn={(columnId, updates) =>
                handleUpdateColumn(board.id, columnId, updates)
              }
              onDeleteColumn={(columnId) => handleDeleteColumn(board.id, columnId)}
              onReorderColumns={(columns) => handleReorderColumns(board.id, columns)}
              onSaveBoard={() => handleSaveBoard(board.id)}
            />
          ))}
        </div>
      </SettingsSection>
    </div>
  );
}
