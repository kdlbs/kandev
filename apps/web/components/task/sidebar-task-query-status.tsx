import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";

export function SidebarTaskQueryStatus({
  error,
  pending,
  hasPage,
  canRetry = true,
  onRetry,
  touchTargets = false,
}: {
  error: string | null;
  pending: boolean;
  hasPage: boolean;
  canRetry?: boolean;
  onRetry: () => void;
  touchTargets?: boolean;
}) {
  const { t } = useTranslation();
  if (error)
    return (
      <div
        role="alert"
        className="flex items-center justify-between gap-2 px-2 py-2 text-xs"
        data-testid="sidebar-task-page-load-error"
      >
        <span className="text-destructive">{error}</span>
        {canRetry && (
          <Button
            variant="link"
            size="sm"
            className={touchTargets ? "min-h-11 min-w-11" : "h-7"}
            onClick={onRetry}
            disabled={pending}
          >
            {t("sidebar:retry")}
          </Button>
        )}
      </div>
    );
  if (!pending || !hasPage) return null;
  return (
    <div role="status" className="px-2 py-1 text-xs text-muted-foreground">
      {t("sidebar:queryRefreshing")}
    </div>
  );
}
