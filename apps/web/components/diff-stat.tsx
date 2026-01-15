'use client';

import { cn } from '@/lib/utils';
import { Badge } from '@kandev/ui/badge';

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
  ahead: 'text-emerald-200 bg-emerald-600/40',
  behind: 'text-yellow-200 bg-yellow-600/40',
};

const lineTone = {
  add: 'text-emerald-600',
  remove: 'text-rose-600',
};

export function CommitStatBadge({ label, tone, className }: CommitStatBadgeProps) {
  return (
    <Badge
      variant="secondary"
      className={cn('border-transparent px-2 text-xs font-semibold rounded-lg', commitTone[tone], className)}
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
