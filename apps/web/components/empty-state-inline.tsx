import Link from 'next/link';

interface EmptyStateInlineProps {
  message: string;
  linkText: string;
  linkHref: string;
}

export function EmptyStateInline({ message, linkText, linkHref }: EmptyStateInlineProps) {
  return (
    <div className="flex h-7 items-center justify-center gap-2 rounded-sm border border-input px-3 text-xs text-muted-foreground">
      <span>{message}</span>
      <Link href={linkHref} className="text-primary hover:underline">
        {linkText}
      </Link>
    </div>
  );
}

