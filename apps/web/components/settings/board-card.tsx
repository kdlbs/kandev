'use client';

import { useMemo, useState, useSyncExternalStore } from 'react';
import {
  DndContext,
  closestCenter,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  IconGripVertical,
  IconTrash,
  IconPlus,
} from '@tabler/icons-react';
import { Card, CardContent } from '@kandev/ui/card';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@kandev/ui/dialog';
import type { Board, Column } from '@/lib/types/http';
import { useRequest } from '@/lib/http/use-request';
import { useToast } from '@/components/toast-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';

type BoardCardProps = {
  board: Board;
  columns: Column[];
  isBoardDirty: boolean;
  areColumnsDirty: boolean;
  onUpdateBoard: (updates: { name?: string; description?: string }) => void;
  onDeleteBoard: () => Promise<unknown>;
  onCreateColumn: (column: Omit<Column, 'id' | 'board_id' | 'created_at' | 'updated_at'>) => void;
  onUpdateColumn: (columnId: string, updates: Partial<Column>) => void;
  onDeleteColumn: (columnId: string) => Promise<unknown>;
  onReorderColumns: (columns: Column[]) => void;
  onSaveBoard: () => Promise<unknown>;
};

const COLOR_OPTIONS = [
  { value: 'bg-neutral-400', label: 'Gray' },
  { value: 'bg-red-500', label: 'Red' },
  { value: 'bg-orange-500', label: 'Orange' },
  { value: 'bg-yellow-500', label: 'Yellow' },
  { value: 'bg-green-500', label: 'Green' },
  { value: 'bg-blue-500', label: 'Blue' },
  { value: 'bg-indigo-500', label: 'Indigo' },
  { value: 'bg-violet-500', label: 'Violet' },
  { value: 'bg-cyan-500', label: 'Cyan' },
  { value: 'bg-sky-500', label: 'Sky' },
];

function SortableColumnRow({
  column,
  onUpdate,
  onDelete,
}: {
  column: Column;
  onUpdate: (updates: Partial<Column>) => void;
  onDelete: () => Promise<unknown>;
}) {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({
    id: column.id,
  });
  const { toast } = useToast();
  const deleteRequest = useRequest(onDelete);

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  const handleNameChange = (value: string) => {
    onUpdate({ name: value });
  };

  const handleColorChange = (value: string) => {
    onUpdate({ color: value });
  };

  const handleDelete = async () => {
    try {
      await deleteRequest.run();
    } catch (error) {
      toast({
        title: 'Failed to delete column',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  return (
    <div ref={setNodeRef} style={style} className="flex gap-2 items-start">
      <button
        type="button"
        className="mt-2 p-1 rounded-md text-muted-foreground hover:text-foreground cursor-grab"
        {...attributes}
        {...listeners}
      >
        <IconGripVertical className="h-4 w-4" />
      </button>
      <div className="flex-1 grid grid-cols-[1fr_1fr_auto] gap-2 items-start">
        <div className="space-y-1">
          <Input
            placeholder="Column title"
            value={column.name}
            onChange={(e) => handleNameChange(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <select
            value={column.color}
            onChange={(e) => handleColorChange(e.target.value)}
            className="w-full h-9 px-3 rounded-md border border-input bg-background text-sm"
          >
            {COLOR_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={handleDelete}
          disabled={deleteRequest.isLoading}
          className="mt-1"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

export function BoardCard({
  board,
  columns,
  isBoardDirty,
  areColumnsDirty,
  onUpdateBoard,
  onDeleteBoard,
  onCreateColumn,
  onUpdateColumn,
  onDeleteColumn,
  onReorderColumns,
  onSaveBoard,
}: BoardCardProps) {
  const columnItems = useMemo(() => columns.map((column) => column.id), [columns]);
  const { toast } = useToast();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const isMounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false
  );

  const saveBoardRequest = useRequest(onSaveBoard);
  const deleteBoardRequest = useRequest(onDeleteBoard);

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = columns.findIndex((column) => column.id === active.id);
    const newIndex = columns.findIndex((column) => column.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    const nextColumns = arrayMove(columns, oldIndex, newIndex).map((column, index) => ({
      ...column,
      position: index,
    }));
    onReorderColumns(nextColumns);
  };

  const handleAddColumn = async () => {
    const lastColor = columns[columns.length - 1]?.color ?? COLOR_OPTIONS[0].value;
    const lastIndex = COLOR_OPTIONS.findIndex((option) => option.value === lastColor);
    const nextColor =
      lastIndex >= 0 ? COLOR_OPTIONS[(lastIndex + 1) % COLOR_OPTIONS.length].value : COLOR_OPTIONS[0].value;
    onCreateColumn({
      name: '',
      position: columns.length,
      state: 'TODO',
      color: nextColor,
    });
  };

  const handleDeleteBoard = async () => {
    try {
      await deleteBoardRequest.run();
      setDeleteOpen(false);
    } catch (error) {
      toast({
        title: 'Failed to delete board',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleSaveBoard = async () => {
    try {
      await saveBoardRequest.run();
    } catch (error) {
      toast({
        title: 'Failed to save board changes',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <div className="space-y-2 flex-1">
              <Label className="flex items-center gap-2">
                <span>Board Name</span>
                {isBoardDirty && <UnsavedChangesBadge />}
              </Label>
              <div className="flex items-center gap-2">
                <Input
                  value={board.name}
                  onChange={(e) => onUpdateBoard({ name: e.target.value })}
                />
                <UnsavedSaveButton
                  isDirty={isBoardDirty || areColumnsDirty}
                  isLoading={saveBoardRequest.isLoading}
                  status={saveBoardRequest.status}
                  onClick={handleSaveBoard}
                />
              </div>
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Label className="flex items-center gap-2">
                  <span>Columns</span>
                  {areColumnsDirty && <UnsavedChangesBadge />}
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleAddColumn}
                >
                  <IconPlus className="h-4 w-4 mr-1" />
                  Add Column
                </Button>
              </div>
            </div>
            <div className="space-y-2">
              {isMounted ? (
                <DndContext collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
                  <SortableContext items={columnItems}>
                    {columns.map((column) => (
                      <SortableColumnRow
                        key={column.id}
                        column={column}
                        onUpdate={(updates) => onUpdateColumn(column.id, updates)}
                        onDelete={() => onDeleteColumn(column.id)}
                      />
                    ))}
                  </SortableContext>
                </DndContext>
              ) : (
                columns.map((column) => (
                  <SortableColumnRow
                    key={column.id}
                    column={column}
                    onUpdate={(updates) => onUpdateColumn(column.id, updates)}
                    onDelete={() => onDeleteColumn(column.id)}
                  />
                ))
              )}
            </div>
          </div>
          <div className="flex justify-end">
            <Button
              type="button"
              variant="destructive"
              onClick={() => setDeleteOpen(true)}
              disabled={deleteBoardRequest.isLoading}
            >
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Board
            </Button>
          </div>
        </div>
      </CardContent>
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete board</DialogTitle>
            <DialogDescription>
              This will remove the board and its columns. Tasks will not be deleted. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteOpen(false)}>
              Cancel
            </Button>
            <Button type="button" variant="destructive" onClick={handleDeleteBoard}>
              Delete Board
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
