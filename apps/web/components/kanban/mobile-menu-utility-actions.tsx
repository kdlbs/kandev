"use client";

import {
  IconActivity,
  IconAlertTriangle,
  IconMoon,
  IconStethoscope,
  IconSun,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { DestinationRows } from "@/components/navigation/destination-rows";
import { useStaticDestinations } from "@/hooks/use-app-destinations";
import { MOBILE_MENU_UTILITY_SECTIONS } from "@/lib/navigation/surface-policy";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { useConnectionIssueCopy } from "@/components/app-status-bar/connection-status-item";
import { useTheme } from "@/components/theme/app-theme";
import { getThemeToggleLabelKey, getThemeToggleTarget } from "@/components/theme/theme-toggle";
import { cn } from "@/lib/utils";
import {
  mobileControlClass,
  mobileControlIconClass,
  mobileSectionTitleClass,
} from "./mobile-menu-styles";

export function MobileUtilityActions({
  showHealthIndicator,
  onOpenHealthDialog,
  onOpenImproveKandev,
  onOpenChange,
}: {
  showHealthIndicator: boolean;
  onOpenHealthDialog: () => void;
  onOpenImproveKandev: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const { enabled: statusDrawerEnabled, issueSeverity, openStatusDrawer } = useAppStatusDrawer();
  const issueDetails = useConnectionIssueCopy(issueSeverity);
  const { resolvedTheme, setTheme } = useTheme();
  const themeToggleTarget = getThemeToggleTarget(resolvedTheme);
  // Stats, Settings, and anything else added to the manifest's insight/utility
  // sections. Before the manifest, this block hardcoded a Settings link and
  // Stats had no phone entry point at all.
  const destinations = useStaticDestinations("mobileMenu", MOBILE_MENU_UTILITY_SECTIONS);
  const closeSheet = () => onOpenChange(false);
  const openHealth = () => {
    closeSheet();
    onOpenHealthDialog();
  };
  const openStatus = () => {
    closeSheet();
    requestAnimationFrame(openStatusDrawer);
  };

  return (
    <div className="mt-auto flex flex-col gap-3 pt-4 border-t border-border">
      <div className={mobileSectionTitleClass}>{t("kanban:utilities")}</div>
      {statusDrawerEnabled && (
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
          aria-label={issueDetails?.description}
          data-connection-severity={issueSeverity === "none" ? undefined : issueSeverity}
        >
          <IconActivity className={mobileControlIconClass} />
          {t("common:status")}
          {issueDetails && (
            <span
              className={cn("ml-auto size-2 rounded-full", issueDetails.dotClass)}
              aria-hidden="true"
            />
          )}
        </Button>
      )}
      <Button
        type="button"
        variant="outline"
        className={cn(mobileControlClass, "h-11 cursor-pointer justify-start gap-3")}
        onClick={() => setTheme(themeToggleTarget)}
        data-testid="mobile-theme-toggle-button"
        aria-label={t(getThemeToggleLabelKey(resolvedTheme))}
        aria-pressed={resolvedTheme === "dark"}
      >
        {resolvedTheme === "dark" ? (
          <IconSun className={mobileControlIconClass} />
        ) : (
          <IconMoon className={mobileControlIconClass} />
        )}
        {t("sidebar:toggleTheme")}
      </Button>
      <DestinationRows
        destinations={destinations}
        onNavigate={closeSheet}
        className="gap-3 px-3 text-sm"
      />
      <Button
        type="button"
        variant="outline"
        className="h-11 w-full cursor-pointer justify-start gap-3 px-3 text-sm"
        onClick={onOpenImproveKandev}
        data-testid="mobile-improve-kandev-button"
      >
        <IconStethoscope className={mobileControlIconClass} />
        {t("kanban:improveKandev")}
      </Button>
      {showHealthIndicator && (
        <Button
          type="button"
          variant="outline"
          className={cn(mobileControlClass, "cursor-pointer justify-start gap-3")}
          onClick={openHealth}
        >
          <IconAlertTriangle className={cn(mobileControlIconClass, "text-warning")} />
          {t("kanban:healthIssues")}
        </Button>
      )}
    </div>
  );
}
