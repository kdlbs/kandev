import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronRight, IconLayoutGrid } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { workspaceCanvasSettingsHref } from "@/lib/api/domains/canvas-api";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";

export function MobileCanvasesSection({
  workspaceId,
  entries,
  loading,
  error,
  onRetry,
  onNavigate,
}: {
  workspaceId: string;
  entries: ShortcutCatalogEntry[];
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  const enabled = useFeature("canvases");
  const [expanded, setExpanded] = useState(false);
  const bodyId = useId();
  if (!enabled) return null;
  return (
    <section className="space-y-2 border-t border-border pt-2">
      <Button
        variant="ghost"
        className="h-11 w-full cursor-pointer justify-start gap-2 px-0 text-sm font-medium hover:bg-transparent aria-expanded:bg-transparent"
        aria-expanded={expanded}
        aria-controls={bodyId}
        onClick={() => setExpanded(!expanded)}
      >
        {t("canvases:canvases")}
        {expanded ? (
          <IconChevronDown className="size-3.5 text-muted-foreground" />
        ) : (
          <IconChevronRight className="size-3.5 text-muted-foreground" />
        )}
      </Button>
      {expanded && (
        <div id={bodyId} className="space-y-2">
          {loading && (
            <p role="status" className="text-sm text-muted-foreground">
              {t("common:loading")}
            </p>
          )}
          {error && (
            <div role="alert" className="space-y-2 text-sm">
              <p>{t("common:requestFailed")}</p>
              <Button variant="outline" className="h-11 cursor-pointer" onClick={onRetry}>
                {t("canvases:retry")}
              </Button>
            </div>
          )}
          {!loading &&
            !error &&
            entries.map((entry) => (
              <Button
                key={entry.target.id}
                asChild
                variant="outline"
                className="h-11 w-full cursor-pointer justify-start gap-3 px-3"
              >
                <Link href={entry.href!} onClick={onNavigate}>
                  <IconLayoutGrid className="size-4 shrink-0" />
                  <span className="min-w-0 truncate">{entry.label}</span>
                </Link>
              </Button>
            ))}
          <Button
            asChild
            variant="outline"
            className="h-11 w-full cursor-pointer justify-start gap-3 px-3"
          >
            <Link href={workspaceCanvasSettingsHref(workspaceId)} onClick={onNavigate}>
              <IconLayoutGrid className="size-4 shrink-0" />
              {t("canvases:openWorkspaceSettings")}
            </Link>
          </Button>
        </div>
      )}
    </section>
  );
}
