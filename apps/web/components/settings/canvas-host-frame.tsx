"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { PageShell } from "@/components/page-shell";
import { PanelRoot } from "@/components/task/panel-primitives";
import { CanvasHostHeader } from "./canvas-host-components";

export function CanvasHostFrame({
  embedded,
  isMobile,
  title,
  menuOpen,
  setMenuOpen,
  desktopActions,
  desktopOverflowActions,
  desktopOverflowMenuItems,
  desktopOverflowPrimaryAction,
  mobileActionsButton,
  canvasBody,
  mobileActions,
  dialogs,
}: {
  embedded: boolean;
  isMobile: boolean;
  title: string;
  menuOpen: boolean;
  setMenuOpen: (open: boolean) => void;
  desktopActions: ReactNode;
  desktopOverflowActions: ReactNode;
  desktopOverflowMenuItems: ReactNode;
  desktopOverflowPrimaryAction: ReactNode;
  mobileActionsButton: ReactNode;
  canvasBody: ReactNode;
  mobileActions: ReactNode;
  dialogs: ReactNode;
}) {
  const { t } = useTranslation();
  if (embedded) {
    return (
      <PanelRoot data-testid="canvas-host-panel">
        <CanvasHostHeader
          title={title}
          isMobile={isMobile}
          menuOpen={menuOpen}
          onOpenActions={() => setMenuOpen(true)}
          actions={isMobile ? null : desktopActions}
          overflowActions={isMobile ? null : desktopOverflowActions}
        />
        {canvasBody}
        {mobileActions}
        {dialogs}
      </PanelRoot>
    );
  }

  return (
    <PageShell
      title={title}
      backHref="/"
      backLabel={t("sidebar:home")}
      scroll="none"
      topbarTestId="canvas-host-header"
      actions={isMobile ? mobileActionsButton : desktopActions}
      overflowMenuItems={isMobile ? null : desktopOverflowMenuItems}
      overflowPrimaryAction={isMobile ? null : desktopOverflowPrimaryAction}
      contentTestId="canvas-route-content"
      showNavTrigger
    >
      {canvasBody}
      {mobileActions}
      {dialogs}
    </PageShell>
  );
}
