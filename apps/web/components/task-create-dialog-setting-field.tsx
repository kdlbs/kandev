import { type ReactNode } from "react";

export const TASK_CREATE_SETTING_ROW =
  "grid min-w-0 gap-y-1 md:grid-cols-[var(--task-setting-label-width)_minmax(0,var(--task-setting-control-width))] md:items-center md:gap-x-4";
export const TASK_CREATE_SETTING_LABEL =
  "flex min-w-0 items-center gap-1 text-xs font-medium text-foreground md:min-h-7";
export const TASK_CREATE_SETTING_TRIGGER =
  "w-full min-w-0 rounded-md border border-input bg-background/40 font-normal hover:bg-muted/40 focus-visible:ring-muted-foreground/70";

export function TaskCreateSettingField({
  label,
  labelTestId,
  testId,
  controlTestId,
  children,
}: {
  label: ReactNode;
  labelTestId?: string;
  testId?: string;
  controlTestId?: string;
  children: ReactNode;
}) {
  return (
    <div className={TASK_CREATE_SETTING_ROW} data-testid={testId}>
      <div className={TASK_CREATE_SETTING_LABEL} data-testid={labelTestId}>
        {label}
      </div>
      <div className="min-w-0" data-testid={controlTestId}>
        {children}
      </div>
    </div>
  );
}
