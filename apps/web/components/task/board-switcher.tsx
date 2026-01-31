'use client';

import { IconCheck, IconChevronDown } from '@tabler/icons-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { cn } from '@/lib/utils';

type Board = {
  id: string;
  name: string;
};

type BoardSwitcherProps = {
  boards: Board[];
  activeBoardId: string | null;
  onSelect: (boardId: string) => void;
};

export function BoardSwitcher({
  boards,
  activeBoardId,
  onSelect,
}: BoardSwitcherProps) {
  const selectedBoard = boards.find((b) => b.id === activeBoardId);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          title="Switch Boards"
          className={cn(
            'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-xs cursor-pointer',
            'text-foreground hover:bg-foreground/5 transition-colors duration-150',
            'focus:outline-none focus-visible:ring-2 focus-visible:ring-ring'
          )}
        >
          {/* Board Avatar */}
          <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-foreground/10 text-xs font-medium">
            {selectedBoard?.name?.charAt(0) || 'B'}
          </span>
          {/* Board Name */}
          <span className="flex-1 truncate text-left font-medium">
            {selectedBoard?.name || 'Select board'}
          </span>
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 opacity-50" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {boards.map((board) => (
          <DropdownMenuItem
            key={board.id}
            onClick={() => onSelect(board.id)}
            className={cn(
              'justify-between',
              activeBoardId === board.id && 'bg-foreground/10'
            )}
          >
            <div className="flex items-center gap-2">
              <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-foreground/10 text-xs font-medium">
                {board.name.charAt(0)}
              </span>
              <span className="truncate">{board.name}</span>
            </div>
            {activeBoardId === board.id && (
              <IconCheck className="h-4 w-4 shrink-0" />
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
