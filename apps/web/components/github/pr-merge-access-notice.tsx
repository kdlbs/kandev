import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import type { MergeAccessSnapshot } from "@/hooks/domains/github/use-repo-merge-access";

export function PRMergeAccessNotice({
  state,
  actor,
  repository,
  onRetry,
  compact = false,
}: {
  state: Exclude<MergeAccessSnapshot["state"], "allowed">;
  actor: string | null;
  repository: string;
  onRetry: () => void;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const message =
    state === "denied"
      ? t(actor ? "github:mergeAccessDeniedByActor" : "github:mergeAccessDenied", {
          actor,
          repository,
        })
      : t(state === "loading" ? "github:mergeAccessLoading" : "github:mergeAccessUnavailable");
  return (
    <div
      data-testid="pr-merge-access"
      className={`flex min-w-0 flex-col items-start gap-2 text-xs text-muted-foreground ${compact ? "px-1" : ""}`}
    >
      <p role="status" className="max-w-full first-letter:uppercase [overflow-wrap:anywhere]">
        {message}
      </p>
      {state === "unknown" && (
        <Button
          variant="outline"
          className={controlSizingClassName("standard", "cursor-pointer")}
          onClick={(event) => {
            event.stopPropagation();
            onRetry();
          }}
        >
          {t("github:checkMergeAccess")}
        </Button>
      )}
    </div>
  );
}
