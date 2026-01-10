'use client';

import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';

type CommitStatBadgeProps = {
  label: string;
  tone: 'ahead' | 'behind';
  className?: string;
};

type LineStatProps = {
  added?: number;
  removed?: number;
  className?: string;
};

const commitTone = {
  ahead: 'text-emerald-600',
  behind: 'text-yellow-600',
};

const lineTone = {
  add: 'text-emerald-600',
  remove: 'text-rose-600',
};

export function CommitStatBadge({ label, tone, className }: CommitStatBadgeProps) {
  return (
    <Badge
      variant="secondary"
      className={cn(
        'bg-transparent border-transparent px-1 text-xs font-semibold',
        commitTone[tone],
        className
      )}
    >
      {label}
    </Badge>
  );
}

export function LineStat({ added, removed, className }: LineStatProps) {
  return (
    <span className={cn('inline-flex items-center gap-2 text-xs font-semibold', className)}>
      {typeof added === 'number' && <span className={lineTone.add}>+{added}</span>}
      {typeof removed === 'number' && <span className={lineTone.remove}>-{removed}</span>}
    </span>
  );
}
