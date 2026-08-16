"use client";

import { useState, type MouseEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@kandev/ui/sheet";
import { MobileWorkspaceActionsSection } from "@/components/app-sidebar/app-sidebar-workspace-actions";
import { AppNavSections, useAppNavDialogs } from "./app-nav-sections";
import { AppNavTrigger } from "./app-nav-trigger";

type AppNavSheetProps = {
  /**
   * In-section navigation rendered above the global sections — the settings
   * tree on /settings, Office's sections on /office. Phone users get the
   * page's own nav plus the app-wide destinations in one place.
   */
  pageNav?: ReactNode | ((close: () => void) => ReactNode);
  /** Manifest destination ids `pageNav` already covers. See `AppNavSections`. */
  omitDestinations?: string[];
};

/**
 * The app navigation sheet behind every `PageShell` hamburger: one trigger,
 * one left-side sheet, one meaning at every width below `md`. Replaces the
 * per-surface menus that each hand-picked a different subset of destinations.
 */
export function AppNavSheet({ pageNav, omitDestinations }: AppNavSheetProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);
  const controls = useAppNavDialogs(close);
  const renderedPageNav = typeof pageNav === "function" ? pageNav(close) : pageNav;

  // pageNav trees (SettingsTree, Office sections) render plain links that
  // don't know about this sheet; close on any link click so navigation isn't
  // hidden behind an open overlay.
  const closeOnLinkClick = (event: MouseEvent<HTMLElement>) => {
    if (event.target instanceof Element && event.target.closest("a[href]")) close();
  };

  return (
    <>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger asChild>
          <AppNavTrigger />
        </SheetTrigger>
        <SheetContent
          side="left"
          className="flex w-80 max-w-[85vw] flex-col overflow-hidden p-0"
          data-testid="app-nav-sheet"
          data-legacy-testid="settings-mobile-menu"
        >
          <SheetHeader className="border-b px-4 py-3 text-left">
            <SheetTitle>{t("common:menu")}</SheetTitle>
          </SheetHeader>
          <nav
            className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto overscroll-contain p-4"
            onClick={closeOnLinkClick}
          >
            {renderedPageNav}
            <AppNavSections
              onNavigate={close}
              omitDestinations={omitDestinations}
              workspaceActions={<MobileWorkspaceActionsSection />}
              controls={controls}
            />
          </nav>
        </SheetContent>
      </Sheet>
      {controls.dialogs}
    </>
  );
}
