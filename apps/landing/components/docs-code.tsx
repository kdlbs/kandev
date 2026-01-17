import type { ReactNode } from 'react';

export function DocsCode({ children }: { children: ReactNode }) {
  return (
    <pre className="mt-4 rounded-xl border border-primary/30 bg-background/80 p-4 text-sm text-foreground shadow-[0_16px_30px_-24px_rgba(59,130,246,0.6)]">
      <code>{children}</code>
    </pre>
  );
}
