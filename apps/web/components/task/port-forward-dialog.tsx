"use client";

import { createContext, useContext, useRef, type ReactNode } from "react";
import { IconNetwork, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useVisualViewportOffset } from "@/hooks/use-visual-viewport-offset";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { PortForwardContent } from "./port-forward-content";
import {
  usePortForwardManagement,
  type PortForwardManagement,
} from "./use-port-forward-management";
import { usePortForwardingVisibility } from "./port-forwarding-visibility-provider";

const PORT_FORWARD_BUTTON_ID = "port-forward-button";
const PORT_FORWARDING_LABEL_KEY = "task:portForwarding";

const ActiveTunnelCountContext = createContext(0);

export function PortForwardButton({ sessionId }: { sessionId?: string | null }) {
  const { t } = useTranslation();
  const { enabled, canToggle, setDialogOpen } = usePortForwardingVisibility();
  const activeCount = useContext(ActiveTunnelCountContext);
  if (!enabled || !canToggle || !sessionId) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          data-testid={PORT_FORWARD_BUTTON_ID}
          variant={activeCount > 0 ? "default" : "outline"}
          aria-label={t(PORT_FORWARDING_LABEL_KEY)}
          className="cursor-pointer px-2 max-md:min-h-11 max-md:min-w-11"
          onClick={(event) => {
            event.currentTarget.focus();
            setDialogOpen(true);
          }}
        >
          <IconNetwork className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {activeCount > 0
          ? t("task:portForwardingTunnelActive", { count: activeCount })
          : t(PORT_FORWARDING_LABEL_KEY)}
      </TooltipContent>
    </Tooltip>
  );
}

/** Keeps the session's port visit outside headers that change at responsive boundaries. */
export function PortForwardingManager({
  sessionId,
  children,
}: {
  sessionId?: string | null;
  children: ReactNode;
}) {
  const { enabled, canToggle, dialogOpen } = usePortForwardingVisibility();
  const active = canToggle && (enabled || dialogOpen);
  const management = usePortForwardManagement(sessionId ?? null, active);
  return (
    <ActiveTunnelCountContext.Provider value={management.activeTunnels.size}>
      {children}
      {active && sessionId && <PortForwardSurface sessionId={sessionId} management={management} />}
    </ActiveTunnelCountContext.Provider>
  );
}

function PortForwardSurface({
  sessionId,
  management,
}: {
  sessionId: string;
  management: PortForwardManagement;
}) {
  const { t } = useTranslation();
  const { dialogOpen, setDialogOpen } = usePortForwardingVisibility();
  const { isMobile, usesDesktopWorkbench } = useResponsiveBreakpoint();
  const opener = useRef<HTMLElement | null>(null);
  const browserAction = useDockviewStore((state) =>
    state.api ? state.openBrowserPanel : undefined,
  );
  const restoreFocus = (event: Event) => {
    event.preventDefault();
    const target = opener.current;
    if (target?.isConnected && target.getClientRects().length) target.focus();
    else
      document
        .querySelector<HTMLElement>(
          `[data-testid="mobile-session-nav-panels"], [data-testid="${PORT_FORWARD_BUTTON_ID}"]`,
        )
        ?.focus();
  };
  const onOpenAutoFocus = () => {
    const element = document.activeElement;
    if (element instanceof HTMLElement && element.dataset.testid === PORT_FORWARD_BUTTON_ID)
      opener.current = element;
    else opener.current = document.querySelector('[data-testid="mobile-session-nav-panels"]');
    if (!management.loaded) void management.refresh();
  };
  const body = (
    <PortForwardContent
      sessionId={sessionId}
      management={management}
      isMobile={isMobile}
      onOpenBrowserPanel={
        usesDesktopWorkbench && browserAction
          ? (url) => {
              browserAction(url);
              setDialogOpen(false);
            }
          : undefined
      }
    />
  );
  if (isMobile) {
    return (
      <PhonePortForwardSurface
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onOpenAutoFocus={onOpenAutoFocus}
        onCloseAutoFocus={restoreFocus}
      >
        {body}
      </PhonePortForwardSurface>
    );
  }
  return (
    <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
      <DialogContent
        data-testid="port-forward-dialog"
        data-presentation="dialog"
        aria-describedby={undefined}
        className="max-h-[calc(100dvh-2rem)] overflow-hidden md:max-w-2xl [&>[data-slot=dialog-close]]:[@media(pointer:coarse)]:size-11"
        onOpenAutoFocus={onOpenAutoFocus}
        onCloseAutoFocus={restoreFocus}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconNetwork className="h-5 w-5" />
            {t(PORT_FORWARDING_LABEL_KEY)}
          </DialogTitle>
        </DialogHeader>
        {body}
      </DialogContent>
    </Dialog>
  );
}

function PhonePortForwardSurface({
  open,
  onOpenChange,
  onOpenAutoFocus,
  onCloseAutoFocus,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onOpenAutoFocus: () => void;
  onCloseAutoFocus: (event: Event) => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const { keyboardOpen, bottomOffset } = useVisualViewportOffset();
  // i18n-exempt: CSS viewport geometry, not user-facing copy.
  const keyboardHeight = `calc(100dvh - ${bottomOffset}px - 1rem)`;
  return (
    <Drawer open={open} onOpenChange={onOpenChange} repositionInputs={false}>
      <DrawerContent
        data-testid="port-forward-dialog"
        data-presentation="drawer"
        aria-describedby={undefined}
        className="h-[calc(100dvh-1rem)] data-[vaul-drawer-direction=bottom]:max-h-[calc(100dvh-1rem)] overflow-hidden outline-none"
        style={
          keyboardOpen
            ? {
                bottom: bottomOffset,
                height: keyboardHeight,
                maxHeight: keyboardHeight,
              }
            : undefined
        }
        onOpenAutoFocus={onOpenAutoFocus}
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <DrawerHeader className="shrink-0 pb-2 text-left">
          <div className="flex min-w-0 items-center justify-between gap-2">
            <DrawerTitle className="flex items-center gap-2">
              <IconNetwork className="h-5 w-5 shrink-0" />
              {t(PORT_FORWARDING_LABEL_KEY)}
            </DrawerTitle>
            <Button
              size="icon"
              variant="ghost"
              className="size-11 shrink-0 cursor-pointer"
              aria-label={t("common:close")}
              onClick={() => onOpenChange(false)}
            >
              <IconX className="h-5 w-5" />
            </Button>
          </div>
        </DrawerHeader>
        {children}
      </DrawerContent>
    </Drawer>
  );
}
