"use client";

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconActivity, IconAlertTriangle, IconStethoscope } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { useConnectionIssueCopy } from "@/components/app-status-bar/connection-status-item";
import { ImproveKandevDialog } from "@/components/improve-kandev-dialog";
import { MobileIntegrationsSection } from "@/components/integrations/integrations-menu";
import { DestinationRows } from "@/components/navigation/destination-rows";
import { MobilePluginNavSection } from "@/components/plugins/mobile-plugin-nav-section";
import { useSystemHealthIndicator } from "@/hooks/use-system-health-indicator";
import { HealthIssuesDialog } from "@/components/system-health/health-indicator";
import { useAppStore } from "@/components/state-provider";
import { useStaticDestinations } from "@/hooks/use-app-destinations";
import { linkToTask } from "@/lib/links";
import { MOBILE_MENU_UTILITY_SECTIONS } from "@/lib/navigation/surface-policy";
import type { NavSection } from "@/lib/navigation/types";
import { useRouter } from "@/lib/routing/client-router";
import { cn } from "@/lib/utils";

/**
 * Dialog-backed rows in the shared nav block (Improve Kandev, Health issues).
 * Hoisted out of `AppNavSections` because the sections render inside a
 * Sheet/Drawer that unmounts on close: a dialog owned there would vanish the
 * moment the menu closes. The host renders `dialogs` as a sibling of its menu
 * surface and passes the rest to `AppNavSections`.
 */
export type AppNavDialogControls = {
  showHealthRow: boolean;
  openImproveKandev: () => void;
  openHealthDialog: () => void;
  dialogs: ReactNode;
};

export function useAppNavDialogs(closeMenu: () => void): AppNavDialogControls {
  const router = useRouter();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const health = useSystemHealthIndicator();
  const [improveOpen, setImproveOpen] = useState(false);

  // Close the menu first, then open the dialog a frame later so the two
  // portals don't fight over focus/scroll locking.
  const openImproveKandev = () => {
    closeMenu();
    requestAnimationFrame(() => setImproveOpen(true));
  };
  const openHealthDialog = () => {
    closeMenu();
    requestAnimationFrame(health.openDialog);
  };

  // Mounted only while open: ImproveKandevDialog runs toast/store hooks on
  // mount, and this hook now sits behind every page's topbar.
  const dialogs = (
    <>
      {improveOpen && (
        <ImproveKandevDialog
          open
          onOpenChange={setImproveOpen}
          workspaceId={workspaceId}
          onSuccess={(task) => router.push(linkToTask(task.id))}
        />
      )}
      {health.dialogOpen && (
        <HealthIssuesDialog
          open
          onOpenChange={(open) => (open ? health.openDialog() : health.closeDialog())}
          issues={health.issues}
        />
      )}
    </>
  );

  return { showHealthRow: health.hasIssues, openImproveKandev, openHealthDialog, dialogs };
}

type AppNavSectionsProps = {
  /** Closes the surrounding sheet/drawer once a destination is opened. */
  onNavigate: () => void;
  /**
   * Sections the caller already covers with its own affordances. The kanban
   * drawer omits "primary": its brand link is Home and its View toggle is
   * Tasks. Everything else draws the full set.
   */
  omitSections?: NavSection[];
  /**
   * Individual manifest destination ids the caller's own nav already covers, for
   * shells that need most of a section. Office omits "tasks": its page nav has a
   * Tasks row pointing inside Office, and the manifest's points at kanban, so
   * both rendered gave two identically labelled rows going to different places.
   */
  omitDestinations?: string[];
  controls: AppNavDialogControls;
};

/**
 * The global navigation block every mobile menu shares: primary destinations,
 * plugin pages, integrations, and the utility tail (Status, Stats, Settings,
 * Improve Kandev, Health issues). All rows come from the navigation manifest,
 * so a new destination appears on every surface at once.
 */
export function AppNavSections({
  onNavigate,
  omitSections = [],
  omitDestinations = [],
  controls,
}: AppNavSectionsProps) {
  const omit = new Set(omitSections);
  return (
    <>
      {!omit.has("primary") && (
        <PrimaryNavSection onNavigate={onNavigate} omitDestinations={omitDestinations} />
      )}
      {!omit.has("plugins") && <MobilePluginNavSection onNavigate={onNavigate} />}
      {!omit.has("integrations") && <MobileIntegrationsSection onNavigate={onNavigate} />}
      <UtilityNavSection onNavigate={onNavigate} controls={controls} />
    </>
  );
}

function PrimaryNavSection({
  onNavigate,
  omitDestinations,
}: {
  onNavigate: () => void;
  omitDestinations: string[];
}) {
  const all = useStaticDestinations("mobileMenu", "primary");
  const destinations = all.filter((destination) => !omitDestinations.includes(destination.id));
  if (destinations.length === 0) return null;
  return (
    <div className="flex flex-col gap-3" data-testid="app-nav-primary">
      <DestinationRows
        destinations={destinations}
        onNavigate={onNavigate}
        className="gap-3 px-3 text-sm"
      />
    </div>
  );
}

function UtilityNavSection({
  onNavigate,
  controls,
}: {
  onNavigate: () => void;
  controls: AppNavDialogControls;
}) {
  const { t } = useTranslation();
  const destinations = useStaticDestinations("mobileMenu", MOBILE_MENU_UTILITY_SECTIONS);
  const utilityRowClass = "h-11 w-full cursor-pointer justify-start gap-3 px-3 text-sm";
  return (
    <div className="mt-auto flex flex-col gap-3 border-t border-border pt-4">
      <div className="text-sm font-medium">{t("common:utilities")}</div>
      <StatusRow closeMenu={onNavigate} />
      <DestinationRows
        destinations={destinations}
        onNavigate={onNavigate}
        className="gap-3 px-3 text-sm"
      />
      <Button
        type="button"
        variant="outline"
        className={utilityRowClass}
        onClick={controls.openImproveKandev}
        data-testid="mobile-improve-kandev-button"
      >
        <IconStethoscope className="h-4 w-4 shrink-0" />
        {t("sidebar:improveKandev")}
      </Button>
      {controls.showHealthRow && (
        <Button
          type="button"
          variant="outline"
          className={utilityRowClass}
          onClick={controls.openHealthDialog}
          data-testid="app-nav-health-button"
        >
          <IconAlertTriangle className="h-4 w-4 shrink-0 text-warning" />
          {t("common:healthIssues")}
        </Button>
      )}
    </div>
  );
}

function StatusRow({ closeMenu }: { closeMenu: () => void }) {
  const { t } = useTranslation();
  const { enabled, issueSeverity, openStatusDrawer } = useAppStatusDrawer();
  const issue = useConnectionIssueCopy(issueSeverity);
  if (!enabled) return null;
  const openStatus = () => {
    closeMenu();
    requestAnimationFrame(openStatusDrawer);
  };
  return (
    <Button
      type="button"
      variant="outline"
      className={cn(
        "relative h-11 w-full cursor-pointer justify-start gap-3 px-3 text-sm",
        issueSeverity === "lost" && "border-destructive/40 text-destructive",
        issueSeverity === "unstable" && "border-amber-500/40 text-amber-500",
      )}
      onClick={openStatus}
      data-testid="mobile-home-status-button"
      // Compose, never replace: a bare `aria-label` of the connection sentence
      // drops the visible "Status" out of the accessible name, so voice control
      // ("click Status") stops matching the button it is looking at. Same shape
      // as the nav trigger's `openNavigationMenuWithConnectionIssue`.
      aria-label={
        issue
          ? t("common:statusWithConnectionIssue", { description: issue.description })
          : undefined
      }
      data-connection-severity={issueSeverity === "none" ? undefined : issueSeverity}
    >
      <IconActivity className="h-4 w-4 shrink-0" />
      {t("common:status")}
      {issue && (
        <span className={cn("ml-auto size-2 rounded-full", issue.dotClass)} aria-hidden="true" />
      )}
    </Button>
  );
}
