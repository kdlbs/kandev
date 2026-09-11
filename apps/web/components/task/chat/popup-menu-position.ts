import { autoUpdate, computePosition, detectOverflow, offset, shift, size } from "@floating-ui/dom";

const MENU_HEIGHT = 280;
const MENU_WIDTH = 420;
const MENU_MARGIN = 8;
const ROW_HEIGHT = 44;
const LIST_PADDING = 8;

/** Owns positioning only while the portal is mounted. All coordinates are client rects. */
export function positionPopupMenu(
  menu: HTMLDivElement,
  getRect: () => DOMRect | null,
  placement: "above" | "below",
) {
  let revision = 0;
  let disposed = false;
  const contextElement = document.activeElement ?? undefined;
  const reference = { contextElement, getBoundingClientRect: () => getRect() ?? new DOMRect() };

  const update = () => {
    if (disposed) return;
    const currentRevision = ++revision;
    const isCurrent = () => !disposed && currentRevision === revision;
    const rect = getRect();
    if (!rect) {
      menu.style.visibility = "hidden";
      return;
    }

    void computePosition({ ...reference, getBoundingClientRect: () => rect }, menu, {
      strategy: "fixed",
      placement: placement === "above" ? "top-start" : "bottom-start",
      middleware: [
        offset(MENU_MARGIN),
        size({
          padding: MENU_MARGIN,
          async apply(state) {
            const overflow = await detectOverflow(state, { padding: MENU_MARGIN });
            if (!isCurrent()) return;
            const viewportWidth = state.rects.floating.width - overflow.left - overflow.right;
            const viewportHeight = state.rects.floating.height - overflow.top - overflow.bottom;
            const headerHeight = menu.firstElementChild?.getBoundingClientRect().height ?? 0;
            const minimumHeight =
              placement === "above" ? headerHeight + ROW_HEIGHT + LIST_PADDING : 0;
            const availableHeight =
              state.availableHeight < minimumHeight && placement === "above"
                ? viewportHeight
                : state.availableHeight;
            Object.assign(menu.style, {
              width: `${Math.max(0, Math.min(MENU_WIDTH, viewportWidth))}px`,
              maxHeight: `${Math.max(0, Math.min(MENU_HEIGHT, viewportHeight, availableHeight))}px`,
            });
          },
        }),
        // Size toward the caret before shifting an occluded anchor into view.
        shift({ padding: MENU_MARGIN, crossAxis: true }),
      ],
    }).then(({ x, y }) => {
      if (!isCurrent()) return;
      Object.assign(menu.style, { left: `${x}px`, top: `${y}px`, visibility: "visible" });
    });
  };

  const stop = autoUpdate(reference, menu, update);
  return () => {
    disposed = true;
    stop();
  };
}
