'use client';

import Link from 'next/link';
import { Button } from '@kandev/ui/button';
import { IconPlus, IconSettings } from '@tabler/icons-react';
import { ConnectionStatus } from '../connection-status';
import { KanbanDisplayDropdown } from '../kanban-display-dropdown';

type KanbanHeaderProps = {
  onCreateTask: () => void;
};

export function KanbanHeader({ onCreateTask }: KanbanHeaderProps) {
  return (
    <header className="flex items-center justify-between p-4 pb-3">
      <div className="flex items-center gap-3">
        <Link href="/" className="text-2xl font-bold hover:opacity-80">
          KanDev.ai
        </Link>
        <ConnectionStatus />
      </div>
      <div className="flex items-center gap-3">
        <Button onClick={onCreateTask} className="cursor-pointer">
          <IconPlus className="h-4 w-4" />
          Add task
        </Button>
        <KanbanDisplayDropdown />
        <Link href="/settings" className="cursor-pointer">
          <Button variant="outline" className="cursor-pointer">
            <IconSettings className="h-4 w-4 mr-2" />
            Settings
          </Button>
        </Link>
      </div>
    </header>
  );
}
