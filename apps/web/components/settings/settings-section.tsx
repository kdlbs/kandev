import { ReactNode } from "react";

type SettingsSectionProps = {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
};

export function SettingsSection({
  icon,
  title,
  description,
  action,
  children,
}: SettingsSectionProps) {
  return (
    <section className="space-y-4">
      {/* Stack the action row under the title on narrow screens instead of
          squeezing both onto one line; shrink-0 keeps the actions from being
          compressed by a long description when side by side. */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            {icon}
            {title}
          </h3>
          {description && <p className="text-sm text-muted-foreground mt-1">{description}</p>}
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
      {children}
    </section>
  );
}
