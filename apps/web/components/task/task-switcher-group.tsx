import { IconChevronDown } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

export function TaskSwitcherSkeleton() {
  return (
    <div className="animate-pulse">
      <div className="h-10 bg-foreground/5" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
      <div className="h-10 bg-foreground/5 mt-px" />
    </div>
  );
}

export function GroupHeader({
  label,
  groupKey,
  count,
  isCollapsed,
  isContinuation = false,
  onToggle,
}: {
  label: string;
  groupKey: string;
  count: number;
  isCollapsed: boolean;
  isContinuation?: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={onToggle}
      data-testid="sidebar-group-header"
      data-group-key={groupKey}
      data-group-label={label}
      className="flex w-full items-center gap-2 bg-background px-3 py-1.5 cursor-pointer hover:bg-foreground/[0.03]"
    >
      <span className="flex-1 truncate text-left text-[12px] font-medium text-foreground/80">
        {label}
      </span>
      {isContinuation && (
        <span className="shrink-0 text-[10px] text-muted-foreground/70">
          {t("sidebar:continuationLabel")}
        </span>
      )}
      <span className="text-[11px] text-muted-foreground/50">{count}</span>
      <IconChevronDown
        className={cn(
          "h-3 w-3 text-muted-foreground/40 transition-transform",
          isCollapsed && "-rotate-90",
        )}
      />
    </button>
  );
}
