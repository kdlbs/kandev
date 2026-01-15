'use client';

import { IconLoader2 } from '@tabler/icons-react';

type TypingIndicatorProps = {
  label?: string;
};

export function TypingIndicator({ label = 'Agent is thinking...' }: TypingIndicatorProps) {
  return (
    <div className="flex items-center gap-2 px-4 py-3 max-w-[85%] rounded-lg bg-muted text-muted-foreground">
      <div className="flex items-center gap-2" role="status" aria-label={label}>
        <IconLoader2 className="h-3 w-3 animate-spin" aria-hidden="true" />
        <span className="text-xs uppercase tracking-wide opacity-70">{label}</span>
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '0ms', animationDuration: '1s' }}
        />
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '150ms', animationDuration: '1s' }}
        />
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '300ms', animationDuration: '1s' }}
        />
      </div>
    </div>
  );
}
