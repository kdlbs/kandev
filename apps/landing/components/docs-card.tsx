import type { ReactNode } from 'react';

export function DocsCard({
  title,
  children,
  variant = 'default',
}: {
  title: string;
  children: ReactNode;
  variant?: 'default' | 'accent' | 'muted';
}) {
  const base =
    'rounded-2xl border border-border/60 bg-card/50 p-6 shadow-[0_12px_40px_-28px_rgba(37,99,235,0.5)] backdrop-blur';
  const variants: Record<typeof variant, string> = {
    default: 'bg-card/50',
    accent: 'border-primary/30 bg-primary/10',
    muted: 'bg-muted/30',
  };

  return (
    <div className={`${base} ${variants[variant]}`}>
      <h2 className="text-lg font-semibold text-foreground">{title}</h2>
      <div className="mt-3 text-sm text-muted-foreground">{children}</div>
    </div>
  );
}
