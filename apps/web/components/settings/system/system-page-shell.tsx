import { ReactNode } from "react";
import { Separator } from "@kandev/ui/separator";
import { SettingsPageHeader } from "@/components/settings/settings-typography";

type SystemPageShellProps = {
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
};

export function SystemPageShell({ title, description, actions, children }: SystemPageShellProps) {
  return (
    <div className="space-y-6" data-testid="system-page-shell">
      <SettingsPageHeader
        title={title}
        description={description}
        actions={actions && <div className="flex flex-col gap-2 md:flex-row">{actions}</div>}
        titleTestId="system-page-title"
      />

      <Separator />

      <div className="space-y-6">{children}</div>
    </div>
  );
}
