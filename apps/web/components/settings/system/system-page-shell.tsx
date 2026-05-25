import { ReactNode } from "react";
import { Separator } from "@kandev/ui/separator";

type SystemPageShellProps = {
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
};

export function SystemPageShell({ title, description, actions, children }: SystemPageShellProps) {
  return (
    <div className="space-y-6" data-testid="system-page-shell">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold" data-testid="system-page-title">
            {title}
          </h2>
          {description && <p className="text-sm text-muted-foreground mt-1">{description}</p>}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>

      <Separator />

      <div className="space-y-6">{children}</div>
    </div>
  );
}
