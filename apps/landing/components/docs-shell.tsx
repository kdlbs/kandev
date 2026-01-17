export function DocsShell({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow: string;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <div className="relative overflow-hidden">
      <div className="absolute  inset-0 -z-10 " />
      <div className="space-y-10">
        <header className="space-y-3">
          <p className="text-sm uppercase tracking-[0.3em] text-muted-foreground">{eyebrow}</p>
          <h1 className="text-4xl font-semibold text-foreground">{title}</h1>
          <p className="text-lg text-muted-foreground">{description}</p>
        </header>
        {children}
      </div>
    </div>
  );
}
