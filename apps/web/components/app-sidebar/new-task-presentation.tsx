import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";

export function NewTaskPresentation({
  compact,
  actions,
  utilities,
  workspaceActions,
  disabled,
  onClick,
}: {
  compact: ReactNode;
  actions: ReactNode;
  utilities: ReactNode;
  workspaceActions: ReactNode;
  disabled: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const fast = useAppStore((s) => s.userSettings.sidebarFastActionsEnabled);
  const style = useAppStore((s) => s.userSettings.sidebarNewTaskStyle);
  return (
    <div className="min-w-0 space-y-1">
      <div className="flex min-w-0 flex-wrap items-center gap-1">
        {style === "compact" ? (
          compact
        ) : (
          <Button
            variant="secondary"
            disabled={disabled}
            onClick={onClick}
            data-testid="create-task-button"
            className="h-7 min-w-[6.5rem] flex-1 gap-2 px-2 py-0 text-[13px] [@media(pointer:coarse)]:min-h-11"
          >
            <IconPlus className="size-4" />
            <span className="truncate">{t("sidebar:newTask")}</span>
          </Button>
        )}
        {fast && actions}
        {workspaceActions}
      </div>
      {!fast && utilities}
    </div>
  );
}

export function LabelledUtility({
  icon,
  label,
  onClick,
  testId,
}: {
  icon: ReactNode;
  label: string;
  onClick: () => void;
  testId: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      data-testid={testId}
      aria-label={label}
      className="flex h-7 min-w-0 flex-1 items-center justify-center gap-2 rounded text-[12px] text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [@media(pointer:coarse)]:min-h-11"
    >
      {icon}
      <span className="truncate">{label}</span>
    </button>
  );
}
