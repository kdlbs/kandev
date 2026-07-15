const DISMISSIBLE_OVERLAY_SLOTS = new Set([
  "context-menu-content",
  "dialog-content",
  "drawer-content",
  "dropdown-menu-content",
  "menubar-content",
  "popover-content",
  "select-content",
  "sheet-content",
]);

const ELIGIBLE_DOCUMENT_COMPONENTS = new Set([
  "browser",
  "commit-detail",
  "diff-viewer",
  "file-editor",
]);

const ALERT_OVERLAY_SLOT = "alert-dialog-content";
const OVERLAY_SELECTOR = "[data-slot]";

type ContextualPanel = {
  api: {
    component: string;
    close: () => void;
  };
};

export type ContextualDockApi = {
  activePanel: ContextualPanel | undefined | null;
};

export type ContextualCloseResult = "overlay" | "blocked" | "document" | "none";

function isOpenOverlay(element: Element): element is HTMLElement {
  if (!(element instanceof HTMLElement)) return false;
  if (element.dataset.state === "closed") return false;
  return element.dataset.state === "open" || element.hasAttribute("data-open");
}

function overlayZIndex(element: HTMLElement): number {
  const value = element.ownerDocument.defaultView?.getComputedStyle(element).zIndex ?? "";
  const parsed = Number.parseInt(value, 10);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function findTopOverlay(root: Document): HTMLElement | null {
  const overlays = Array.from(root.querySelectorAll(OVERLAY_SELECTOR)).filter(
    (element): element is HTMLElement => {
      if (!isOpenOverlay(element)) return false;
      const slot = element.dataset.slot ?? "";
      return slot === ALERT_OVERLAY_SLOT || DISMISSIBLE_OVERLAY_SLOTS.has(slot);
    },
  );
  return overlays.reduce<HTMLElement | null>((top, current) => {
    if (!top) return current;
    return overlayZIndex(current) >= overlayZIndex(top) ? current : top;
  }, null);
}

function dismissOverlay(overlay: HTMLElement): void {
  const KeyboardEventConstructor = overlay.ownerDocument.defaultView?.KeyboardEvent;
  if (!KeyboardEventConstructor) return;
  overlay.dispatchEvent(
    new KeyboardEventConstructor("keydown", {
      bubbles: true,
      cancelable: true,
      code: "Escape",
      key: "Escape",
    }),
  );
}

function hasEditableFocus(root: Document): boolean {
  const active = root.activeElement;
  if (!(active instanceof HTMLElement)) return false;
  return (
    active.isContentEditable ||
    active instanceof HTMLInputElement ||
    active instanceof HTMLTextAreaElement ||
    active instanceof HTMLSelectElement
  );
}

export function closeDesktopContext(
  root: Document,
  dockApi: ContextualDockApi | null,
): ContextualCloseResult {
  if (hasEditableFocus(root)) return "none";
  const overlay = findTopOverlay(root);
  if (overlay) {
    const slot = overlay.dataset.slot ?? "";
    if (slot === ALERT_OVERLAY_SLOT) return "blocked";
    if (DISMISSIBLE_OVERLAY_SLOTS.has(slot)) dismissOverlay(overlay);
    return DISMISSIBLE_OVERLAY_SLOTS.has(slot) ? "overlay" : "blocked";
  }

  const panel = dockApi?.activePanel;
  if (!panel || !ELIGIBLE_DOCUMENT_COMPONENTS.has(panel.api.component)) return "none";
  panel.api.close();
  return "document";
}
