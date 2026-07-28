"use client";

import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import {
  pluginModalManager,
  usePluginModals,
  type OpenPluginModal,
} from "@/lib/plugins/modal-manager";
import type { PluginModalOptions } from "@/lib/plugins/types";
import { PluginErrorBoundary } from "./plugin-error-boundary";

/** Maps `PluginModalOptions.size` to the host's Dialog width classes. */
const SIZE_CLASSES: Record<NonNullable<PluginModalOptions["size"]>, string> = {
  sm: "sm:max-w-sm",
  md: "sm:max-w-xl",
  lg: "sm:max-w-3xl",
  xl: "sm:max-w-5xl",
};

function preventWhenNotDismissible(dismissible: boolean) {
  return (event: Event) => {
    if (!dismissible) event.preventDefault();
  };
}

function PluginModalInstance({ modal }: { modal: OpenPluginModal }) {
  const { instanceId, pluginId, options } = modal;
  const dismissible = options.dismissible ?? true;
  const Content = options.content;
  const guardClose = preventWhenNotDismissible(dismissible);

  const handleOpenChange = (open: boolean) => {
    if (open || !dismissible) return;
    pluginModalManager.close(instanceId);
  };

  return (
    <Dialog open onOpenChange={handleOpenChange}>
      <DialogContent
        className={SIZE_CLASSES[options.size ?? "md"]}
        showCloseButton={dismissible}
        onEscapeKeyDown={guardClose}
        onInteractOutside={guardClose}
      >
        {options.title && (
          <DialogHeader>
            <DialogTitle>{options.title}</DialogTitle>
          </DialogHeader>
        )}
        <PluginErrorBoundary context={`modal "${instanceId}" (plugin "${pluginId}")`}>
          <Content />
        </PluginErrorBoundary>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Renders every open plugin modal (`host.openModal(...)`) in a `@kandev/ui`
 * `Dialog`, each isolated behind its own `PluginErrorBoundary`. Mounted once
 * at the app root, alongside `<PluginBootBridge/>`.
 */
export function PluginModalHost() {
  const modals = usePluginModals();
  if (modals.length === 0) return null;
  return (
    <>
      {modals.map((modal) => (
        <PluginModalInstance key={modal.instanceId} modal={modal} />
      ))}
    </>
  );
}
