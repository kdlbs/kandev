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

function preventWhenNotDismissible(dismissible: boolean) {
  return (event: Event) => {
    if (!dismissible) event.preventDefault();
  };
}

function pluginDialogLabel(): string {
  return t("plugins:pluginDialog");
}

type ModalSurfaceProps = {
  modal: OpenPluginModal;
  dismissible: boolean;
  onOpenChange(open: boolean): void;
};

function PluginDrawer({ modal, dismissible, onOpenChange }: ModalSurfaceProps) {
  const { instanceId, pluginId, options } = modal;
  const Content = options.content;
  const noDescriptionProps = options.description ? {} : { "aria-describedby": undefined };
  return (
    <Drawer open dismissible={dismissible} onOpenChange={onOpenChange}>
      <DrawerContent
        {...noDescriptionProps}
        className="max-h-[90dvh] pb-[max(1rem,env(safe-area-inset-bottom))]"
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

function PluginDialog({ modal, dismissible, onOpenChange }: ModalSurfaceProps) {
  const { instanceId, pluginId, options } = modal;
  const Content = options.content;
  const guardClose = preventWhenNotDismissible(dismissible);
  const noDescriptionProps = options.description ? {} : { "aria-describedby": undefined };
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent
        {...noDescriptionProps}
        data-testid={`plugin-modal-dialog-${instanceId}`}
        data-layout="contained"
        className={
          modal.layout === "task-link"
            ? `${DIALOG_CONTAINMENT_CLASSES} w-[calc(100vw-2rem)] sm:max-w-lg`
            : `${DIALOG_CONTAINMENT_CLASSES} ${SIZE_CLASSES[options.size ?? "md"]}`
        }
        showCloseButton={dismissible}
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
  const props = { modal, dismissible, onOpenChange: handleOpenChange };
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
