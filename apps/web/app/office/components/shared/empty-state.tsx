interface EmptyStateProps {
  message: string;
  description?: string;
  action?: React.ReactNode;
}

export function EmptyState({ message, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <p className="text-sm text-muted-foreground">{message}</p>
      {description && <p className="text-xs text-muted-foreground mt-1">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
