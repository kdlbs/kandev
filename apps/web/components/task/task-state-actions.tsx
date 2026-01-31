'use client';

import { getTaskStateIcon } from '@/lib/ui/state-icons';
import type { TaskState } from '@/lib/types/http';

type TaskStateActionsProps = {
  state?: TaskState;
  className?: string;
};

export function TaskStateActions({ state, className }: TaskStateActionsProps) {
  return (
    <div className="flex items-center justify-end">
      {getTaskStateIcon(state, className)}
    </div>
  );
}
