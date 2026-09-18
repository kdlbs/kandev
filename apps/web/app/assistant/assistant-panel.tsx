import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
export function AssistantPanel({
  title,
  loading,
  loaded,
  error,
  count,
  nextCursor,
  onRefresh,
  onMore,
  children,
}: {
  title: string;
  loading: boolean;
  loaded: boolean;
  error: unknown;
  count: number;
  nextCursor: string;
  onRefresh: () => void;
  onMore: () => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <section className="space-y-3" aria-label={title}>
      <h2 className="font-semibold">{title}</h2>
      {Boolean(error) && (
        <div role="alert" className="space-y-2 text-sm">
          <p>{t("orchestration:assistantUnavailable")}</p>
          <Button variant="outline" className="cursor-pointer max-md:min-h-11" onClick={onRefresh}>
            {t("task:retry")}
          </Button>
        </div>
      )}
      {!loaded && !error && <p role="status">{t("common:loading")}</p>}
      {loaded && count === 0 && !error && (
        <p className="text-sm text-muted-foreground">{t("orchestration:assistantEmpty")}</p>
      )}
      {children}
      {loaded && count > 0 && (
        <p className="text-xs text-muted-foreground">
          {t("orchestration:assistantLoaded", { count })}
        </p>
      )}
      {nextCursor && (
        <Button
          variant="outline"
          disabled={loading}
          className="cursor-pointer max-md:min-h-11"
          onClick={onMore}
        >
          {t("orchestration:assistantLoadMore")}
        </Button>
      )}
    </section>
  );
}
