"use client";

import { forwardRef, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { IconMenu2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { useConnectionIssueCopy } from "@/components/app-status-bar/connection-status-item";
import { cn } from "@/lib/utils";

type AppNavTriggerProps = ComponentProps<typeof Button>;

/**
 * The phone trigger for the app nav sheet — a hamburger that carries the
 * connection-severity dot, so degraded connectivity is visible before the
 * sheet's Status row is. One copy of the dot logic that used to live in both
 * the settings menu trigger and kanban's mobile header.
 *
 * `md:hidden`: from `md` up the AppSidebar is visible and owns navigation.
 */
export const AppNavTrigger = forwardRef<HTMLButtonElement, AppNavTriggerProps>(
  function AppNavTrigger({ className, ...props }, ref) {
    const { t } = useTranslation();
    const { issueSeverity } = useAppStatusDrawer();
    const issue = useConnectionIssueCopy(issueSeverity);
    return (
      <Button
        ref={ref}
        type="button"
        variant="ghost"
        size="icon"
        {...props}
        className={cn(
          "relative h-11 w-11 cursor-pointer md:hidden",
          issueSeverity === "lost" && "text-destructive",
          issueSeverity === "unstable" && "text-amber-500",
          className,
        )}
        aria-label={
          issue
            ? t("common:openNavigationMenuWithConnectionIssue", { description: issue.description })
            : t("common:openNavigationMenu")
        }
        data-testid="app-nav-trigger"
        data-legacy-testid="settings-mobile-menu-button"
        data-connection-severity={issueSeverity === "none" ? undefined : issueSeverity}
      >
        <IconMenu2 className="h-4 w-4" />
        {issue && (
          <span
            className={cn(
              "absolute right-2 top-2 size-2 rounded-full ring-2 ring-background",
              issue.dotClass,
            )}
            aria-hidden="true"
          />
        )}
      </Button>
    );
  },
);
