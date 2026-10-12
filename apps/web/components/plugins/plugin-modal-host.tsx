"use client";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { TooltipProvider } from "@kandev/ui/tooltip";
import {
  pluginModalManager,
  usePluginModals,
  type OpenPluginModal,
} from "@/lib/plugins/modal-manager";
import type { PluginModalOptions } from "@/lib/plugins/types";
import { t } from "@/lib/i18n";
import { PluginErrorBoundary } from "./plugin-error-boundary";

/** Maps `PluginModalOptions.size` to the host's Dialog width classes. */
const SIZE_CLASSES: Record<NonNullable<PluginModalOptions["size"]>, string> = {
  sm: "sm:max-w-sm",
  md: "sm:max-w-xl",
  lg: "sm:max-w-3xl",
  xl: "sm:max-w-5xl",
};

const DIALOG_CONTAINMENT_CLASSES =
  "max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden";
const OPEN_FOCUS_SURFACE_SELECTOR =
  '[role="dialog"][data-state="open"], [role="menu"][data-state="open"]';

function preventWhenNotDismissible(dismissible: boolean) {
  return (event: Event) => {
    if (!dismissible) event.preventDefault();
  };
}

function pluginDialogLabel(): string {
  return t("plugins:pluginDialog");
}

function isElementUnavailable(element: HTMLElement, checkAriaDisabled = true): boolean {
  if (
    element.hidden ||
    element.inert ||
    element.hasAttribute("inert") ||
    element.getAttribute("aria-hidden") === "true" ||
    (checkAriaDisabled && element.getAttribute("aria-disabled") === "true") ||
    element.matches(":disabled")
  ) {
    return true;
  }

  const computedStyle = element.ownerDocument.defaultView?.getComputedStyle(element);
  return (
    computedStyle?.display === "none" ||
    computedStyle?.visibility === "hidden" ||
    computedStyle?.visibility === "collapse"
  );
}

function isAvailableFocusTarget(element: HTMLElement | null | undefined): element is HTMLElement {
  if (
    !element?.isConnected ||
    element === element.ownerDocument.body ||
    element === element.ownerDocument.documentElement
  ) {
    return false;
  }

  for (let ancestor: HTMLElement | null = element; ancestor; ancestor = ancestor.parentElement) {
    if (isElementUnavailable(ancestor, ancestor === element)) return false;
  }

  return true;
}

function findRenderedPluginModalSurface(instanceId: string): HTMLElement | undefined {
  const surface = document.querySelector<HTMLElement>(
    `[data-plugin-modal-instance="${instanceId}"][data-state="open"]`,
  );
  return isAvailableFocusTarget(surface) ? surface : undefined;
}

function findActiveModalSurface(closingInstanceId: string): HTMLElement | undefined {
  const modals = pluginModalManager.getSnapshot();
  const livePluginModalIds = new Set(
    modals
      .filter((modal) => modal.instanceId !== closingInstanceId)
      .map((modal) => modal.instanceId),
  );
  const pluginSurfaces = Array.from(livePluginModalIds)
    .map(findRenderedPluginModalSurface)
    .filter((surface): surface is HTMLElement => Boolean(surface));
  const openSurfaces = Array.from(
    document.querySelectorAll<HTMLElement>(OPEN_FOCUS_SURFACE_SELECTOR),
  ).filter((surface) => {
    const instanceId = surface.dataset.pluginModalInstance;
    return (
      (!instanceId || livePluginModalIds.has(instanceId)) &&
      instanceId !== closingInstanceId &&
      isAvailableFocusTarget(surface)
    );
  });
  const activeSurfaces = Array.from(new Set([...pluginSurfaces, ...openSurfaces]));
  const activeElement = document.activeElement;
  const focusedSurface =
    activeElement instanceof HTMLElement
      ? activeElement.closest<HTMLElement>(OPEN_FOCUS_SURFACE_SELECTOR)
      : null;
  if (focusedSurface && activeSurfaces.includes(focusedSurface)) return focusedSurface;

  return activeSurfaces
    .sort((left, right) => {
      const relation = left.compareDocumentPosition(right);
      if (relation & Node.DOCUMENT_POSITION_FOLLOWING) return -1;
      if (relation & Node.DOCUMENT_POSITION_PRECEDING) return 1;
      return 0;
    })
    .at(-1);
}

function restoreModalFocus(modal: OpenPluginModal): void {
  if (typeof document === "undefined") return;

  const activeSurface = findActiveModalSurface(modal.instanceId);
  if (activeSurface) {
    const activeElement = document.activeElement;
    if (
      activeElement instanceof HTMLElement &&
      activeSurface.contains(activeElement) &&
      isAvailableFocusTarget(activeElement)
    ) {
      return;
    }

    if (modal.openerElement && activeSurface.contains(modal.openerElement)) {
      if (isAvailableFocusTarget(modal.openerElement)) {
        modal.openerElement.focus({ preventScroll: true });
        return;
      }
    }

    activeSurface.focus({ preventScroll: true });
    return;
  }

  if (isAvailableFocusTarget(modal.openerElement)) {
    modal.openerElement.focus({ preventScroll: true });
  }
}

type ModalSurfaceProps = {
  modal: OpenPluginModal;
  dismissible: boolean;
  onOpenChange(open: boolean): void;
  onCloseAutoFocus(event: Event): void;
};

function PluginDrawer({ modal, dismissible, onOpenChange, onCloseAutoFocus }: ModalSurfaceProps) {
  const { instanceId, pluginId, options } = modal;
  const Content = options.content;
  const noDescriptionProps = options.description ? {} : { "aria-describedby": undefined };
  return (
    <Drawer open dismissible={dismissible} onOpenChange={onOpenChange}>
      <DrawerContent
        {...noDescriptionProps}
        data-plugin-modal-instance={instanceId}
        className="max-h-[90dvh] pb-[max(1rem,env(safe-area-inset-bottom))]"
        onCloseAutoFocus={onCloseAutoFocus}
      >
        {(options.title || options.description) && (
          <DrawerHeader>
            {options.title ? (
              <DrawerTitle>{options.title}</DrawerTitle>
            ) : (
              <DrawerTitle className="sr-only">{pluginDialogLabel()}</DrawerTitle>
            )}
            {options.description && <DrawerDescription>{options.description}</DrawerDescription>}
          </DrawerHeader>
        )}
        {!options.title && !options.description && (
          <DrawerTitle className="sr-only">{pluginDialogLabel()}</DrawerTitle>
        )}
        <div className="min-h-0 overflow-y-auto overscroll-contain px-4 pb-4">
          <PluginErrorBoundary context={`drawer "${instanceId}" (plugin "${pluginId}")`}>
            <Content />
          </PluginErrorBoundary>
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function PluginDialog({ modal, dismissible, onOpenChange, onCloseAutoFocus }: ModalSurfaceProps) {
  const { instanceId, pluginId, options } = modal;
  const Content = options.content;
  const guardClose = preventWhenNotDismissible(dismissible);
  const noDescriptionProps = options.description ? {} : { "aria-describedby": undefined };
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent
        {...noDescriptionProps}
        data-plugin-modal-instance={instanceId}
        data-testid={`plugin-modal-dialog-${instanceId}`}
        data-layout="contained"
        className={
          modal.layout === "task-link"
            ? `${DIALOG_CONTAINMENT_CLASSES} w-[calc(100vw-2rem)] sm:max-w-lg`
            : `${DIALOG_CONTAINMENT_CLASSES} ${SIZE_CLASSES[options.size ?? "md"]}`
        }
        showCloseButton={dismissible}
        onCloseAutoFocus={onCloseAutoFocus}
        onEscapeKeyDown={guardClose}
        onInteractOutside={guardClose}
      >
        {/* Plugin-owned title/description; render either when supplied. */}
        {(options.title || options.description) && (
          <DialogHeader>
            {options.title ? (
              <DialogTitle>{options.title}</DialogTitle>
            ) : (
              <DialogTitle className="sr-only">{pluginDialogLabel()}</DialogTitle>
            )}
            {options.description && <DialogDescription>{options.description}</DialogDescription>}
          </DialogHeader>
        )}
        {!options.title && !options.description && (
          <DialogTitle className="sr-only">{pluginDialogLabel()}</DialogTitle>
        )}
        <div
          data-testid={`plugin-modal-body-${instanceId}`}
          className="row-start-2 min-h-0 min-w-0 overflow-x-hidden overflow-y-auto overscroll-contain"
        >
          <PluginErrorBoundary context={`modal "${instanceId}" (plugin "${pluginId}")`}>
            <Content />
          </PluginErrorBoundary>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function PluginModalInstance({ modal }: { modal: OpenPluginModal }) {
  const dismissible = modal.options.dismissible ?? true;
  const handleOpenChange = (open: boolean) => {
    if (open || !dismissible) return;
    pluginModalManager.close(modal.instanceId);
  };
  const handleCloseAutoFocus = (event: Event) => {
    event.preventDefault();
    restoreModalFocus(modal);
  };
  const props = {
    modal,
    dismissible,
    onOpenChange: handleOpenChange,
    onCloseAutoFocus: handleCloseAutoFocus,
  };
  return modal.options.presentation === "drawer" ? (
    <PluginDrawer {...props} />
  ) : (
    <PluginDialog {...props} />
  );
}

/**
 * Renders every open plugin modal (`host.openModal(...)`) in a `@kandev/ui`
 * `Dialog`, each isolated behind its own `PluginErrorBoundary`. Mounted once
 * inside `<AppShell/>`, where plugin-owned forms inherit the app providers.
 *
 * Keep the local `TooltipProvider` so this host is also safe in isolated
 * mounts and tests. Radix supports nesting it under AppShell's provider.
 */
export function PluginModalHost() {
  const modals = usePluginModals();
  if (modals.length === 0) return null;
  return (
    <TooltipProvider>
      {modals.map((modal) => (
        <PluginModalInstance key={modal.instanceId} modal={modal} />
      ))}
    </TooltipProvider>
  );
}
