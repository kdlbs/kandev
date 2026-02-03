'use client';

import { Button } from '@kandev/ui/button';
import { IconPlus } from '@tabler/icons-react';
import { cn } from '@/lib/utils';

type MobileFabProps = {
  onClick: () => void;
  isDragging?: boolean;
};

export function MobileFab({ onClick, isDragging = false }: MobileFabProps) {
  return (
    <Button
      onClick={onClick}
      size="icon"
      className={cn(
        'fixed z-40 h-14 w-14 rounded-full shadow-lg transition-all duration-200',
        'cursor-pointer hover:scale-105 active:scale-95',
        'right-4'
      )}
      style={{
        // Use calc to add safe area inset to bottom position
        bottom: isDragging
          ? 'calc(8rem + env(safe-area-inset-bottom, 0px))'
          : 'calc(1.5rem + env(safe-area-inset-bottom, 0px))',
        opacity: isDragging ? 0.5 : 1,
      }}
    >
      <IconPlus className="h-6 w-6" />
      <span className="sr-only">Add task</span>
    </Button>
  );
}
