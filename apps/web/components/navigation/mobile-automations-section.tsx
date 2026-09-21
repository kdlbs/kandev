import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronRight, IconListDetails } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useWorkspaceAutomations } from "@/components/runs/use-workspace-automations";
import { useAutomationSummaries } from "@/components/runs/use-automation-summaries";
import { useLiveRefresh } from "@/components/runs/use-live-refresh";
import { buildAutomationRows, STATE_LABEL_KEY } from "@/components/runs/automation-rows";
import { AUTOMATIONS_HREF } from "@/components/runs/runs-view";
import { workspaceSettingsHref } from "@/lib/settings/workspace-settings-tabs";

export function MobileAutomationsSection({
  workspaceId,
  onNavigate,
}: {
  workspaceId: string;
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  return (
    <section
      className="flex flex-col gap-3 border-t border-border pt-4"
      data-testid="mobile-automations-section"
    >
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          className="cursor-pointer h-11 min-w-11 flex-1 justify-start gap-2 px-0 text-sm font-medium hover:bg-transparent aria-expanded:bg-transparent"
          aria-expanded={expanded}
          aria-controls="mobile-automations-body"
          onClick={() => setExpanded(!expanded)}
        >
          {t("automations:automations")}
          {expanded ? (
            <IconChevronDown className="size-3.5 text-muted-foreground" />
          ) : (
            <IconChevronRight className="size-3.5 text-muted-foreground" />
          )}
        </Button>
        <Button
          asChild
          variant="ghost"
          className="cursor-pointer size-11 shrink-0 text-muted-foreground"
        >
          <Link
            href={AUTOMATIONS_HREF}
            onClick={onNavigate}
            aria-label={t("automations:openAutomations")}
          >
            <IconListDetails className="size-4" />
          </Link>
        </Button>
      </div>
      {expanded && (
        <MobileAutomationRows key={workspaceId} workspaceId={workspaceId} onNavigate={onNavigate} />
      )}
    </section>
  );
}

function MobileAutomationRows({
  workspaceId,
  onNavigate,
}: {
  workspaceId: string;
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  const list = useWorkspaceAutomations(workspaceId);
  const activity = useAutomationSummaries(workspaceId);
  useLiveRefresh(true, activity.refresh);
  const rows = buildAutomationRows(list.automations, activity.summaries);
  const pending = list.loading || activity.loading;
  const error = list.error || activity.error;
  return (
    <div id="mobile-automations-body" className="flex flex-col gap-2">
      {pending && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("common:loading")}
        </p>
      )}
      {error && (
        <div role="alert" className="space-y-2 text-sm">
          <p>
            {list.error
              ? t("automations:failedToLoadAutomations")
              : t("automations:failedToLoadAutomationActivity")}
          </p>
          <Button
            variant="outline"
            className="cursor-pointer h-11"
            onClick={() => {
              list.refresh();
              activity.refresh();
            }}
          >
            {t("automations:tryAgain")}
          </Button>
        </div>
      )}
      {!list.loading &&
        !list.error &&
        rows.map(({ automation, state }) => (
          <Button
            asChild
            variant="outline"
            className="cursor-pointer min-h-11 h-auto justify-start gap-3 px-3 py-2"
            key={automation.id}
          >
            <Link href={`${AUTOMATIONS_HREF}/${automation.id}`} onClick={onNavigate}>
              <span className="min-w-0 flex-1 truncate text-left">{automation.name}</span>
              {!activity.loading && !activity.error && (
                <span className="text-xs text-muted-foreground">{t(STATE_LABEL_KEY[state])}</span>
              )}
            </Link>
          </Button>
        ))}
      <Button asChild variant="outline" className="cursor-pointer h-11 justify-start px-3">
        <Link href={workspaceSettingsHref(workspaceId, "automations")} onClick={onNavigate}>
          {t("automations:setUpAnAutomation")}
        </Link>
      </Button>
    </div>
  );
}
