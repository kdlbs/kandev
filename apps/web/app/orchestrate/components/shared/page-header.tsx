interface PageHeaderProps {
  title: string;
  action?: React.ReactNode;
}

export function PageHeader({ title, action }: PageHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">{title}</h1>
      {action}
    </div>
  );
}
