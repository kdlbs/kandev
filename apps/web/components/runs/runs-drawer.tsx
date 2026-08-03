"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconHistory } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
import type { Automation, AutomationRun } from "@/lib/types/automation";
import Link from "@/components/routing/app-link";
import { describeAutomationSchedule, scheduleBinding } from "./automation-schedule";
import { groupRunsByState } from "./run-status";
import { RunGroup } from "./runs-rail";
import { detailTabHref } from "./runs-view";

/**
 * The run switcher, on a phone.
 *
 * The rail is a permanent column sized by dragging its edge, which is a mouse
 * gesture and a fifth of the viewport on a phone — it leaves the transcript and
 * the composer, the two things this page is for, sharing what is left. On
 * mobile the same list moves behind a control in the flow of the page, so the
 * conversation gets the whole screen and the runs are one tap away.
 *
 * Deliberately reuses RunGroup rather than restating the row markup: the two
 * surfaces should never drift on what a run looks like or how it is grouped.
 */
export function RunsDrawer({
  automation,
  runs,
  selectedRunId,
  onSelect,
}: {
  automation: Automation;
  runs: AutomationRun[];
  selectedRunId: string | null;
  onSelect: (run: AutomationRun) => void;
}) {
  const { t } = useTranslation();
  // Controlled, because picking a run only changes a query parameter — the page
  // does not remount, so an uncontrolled drawer would stay open on top of the
  // transcript the user just asked for, and they would have to dismiss it by
  // hand before they could reply.
  const [open, setOpen] = useState(false);
  const { running, completed } = groupRunsByState(runs);

  const selectAndClose = (run: AutomationRun) => {
    setOpen(false);
    onSelect(run);
  };
  const hasSchedule = Boolean(scheduleBinding(automation).expression);

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <div className="flex items-center justify-between gap-2 border-b border-border/60 px-3 py-2">
        <DrawerTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="cursor-pointer gap-1.5 px-2 text-xs"
            data-testid="runs-drawer-trigger"
          >
            <IconHistory className="h-3.5 w-3.5" />
            {t("automations:runsCount", { count: runs.length })}
          </Button>
        </DrawerTrigger>
        <Link
          href={detailTabHref(automation.id, "configure")}
          className="shrink-0 cursor-pointer text-xs text-muted-foreground hover:text-foreground transition-colors"
          data-testid="automation-details-link"
        >
          {t("automations:details")}
        </Link>
      </div>
      <DrawerContent data-testid="runs-drawer">
        <DrawerHeader className="pb-0">
          <DrawerTitle className="text-sm">{t("automations:runs")}</DrawerTitle>
          {/* The schedule is what makes the times below legible, same as the rail. */}
          {hasSchedule && (
            <p className="truncate text-xs text-muted-foreground">
              {describeAutomationSchedule(automation)}
            </p>
          )}
        </DrawerHeader>
        <div className="max-h-[60vh] overflow-y-auto px-1.5 pb-6">
          {runs.length === 0 ? (
            <p
              className="px-2.5 py-6 text-xs text-muted-foreground"
              data-testid="runs-drawer-empty"
            >
              {t("automations:noRunsYet")}
            </p>
          ) : (
            <>
              <RunGroup
                title={t("automations:running")}
                groupId="running"
                runs={running}
                selectedRunId={selectedRunId}
                onSelect={selectAndClose}
              />
              <RunGroup
                title={t("automations:completed")}
                groupId="completed"
                runs={completed}
                selectedRunId={selectedRunId}
                onSelect={selectAndClose}
              />
            </>
          )}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
