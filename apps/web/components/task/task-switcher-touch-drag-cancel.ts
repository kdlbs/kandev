import { useRef } from "react";

/**
 * dnd-kit's TouchSensor arms on touchstart and activates after the 250ms
 * delay — before a long-press (≈700ms) can open this context menu. A
 * stationary long-press therefore starts a row drag that is still live when
 * the menu opens. While a drag is active the TouchSensor listens for
 * `touchcancel` on the element the touch started on, so dispatching one at
 * that element aborts the drag (onDragCancel) instead of dropping it: the
 * row stays put and the menu remains usable. Inert when no touch has started
 * on this row (desktop right-click, or a sensor that already detached after a
 * quick tap), because then nothing listens for the event.
 */
type CancelTouchDrag = (touchStartTarget: EventTarget | null) => void;

const cancelTouchDrag: CancelTouchDrag = (touchStartTarget) => {
  if (touchStartTarget instanceof Element && typeof TouchEvent === "function") {
    touchStartTarget.dispatchEvent(
      new TouchEvent("touchcancel", { bubbles: true, cancelable: true }),
    );
  }
};

/**
 * Coordinates the context menu with the row's touch-drag sensor: remembers
 * the element the touch began on and cancels the in-flight drag when the menu
 * opens. Returns the menu `onOpenChange` handler and the trigger-wrapper
 * capture props.
 */
export function useMenuTouchDragCancel(onOpenChange: (open: boolean) => void) {
  const touchStartRef = useRef<{ target: EventTarget; identifier: number } | null>(null);
  const menuOpenRef = useRef(false);
  const handleOpenChange = (open: boolean) => {
    onOpenChange(open);
    menuOpenRef.current = open;
    if (open) {
      // A touch long-press has already armed the row's TouchSensor (250ms)
      // when the menu opens (~700ms); cancel that drag at the touchstart
      // target so the menu gesture never moves the row.
      const target = touchStartRef.current?.target ?? null;
      touchStartRef.current = null;
      cancelTouchDrag(target);
    } else {
      touchStartRef.current = null;
    }
  };
  return {
    handleOpenChange,
    triggerProps: {
      // The TouchSensor attaches its touchcancel listener to the element the
      // touch began on while a drag is active. Track only the first touch of
      // a single-touch gesture (dnd-kit's TouchSensor rejects multi-touch)
      // and only while the menu is closed, and drop the target when that
      // touch ends or the menu closes — not when another finger lifts — so a
      // later open never dispatches a synthetic touchcancel for a gesture
      // that is no longer active (pull-to-refresh and touch-scroll listen for
      // bubbled touchcancel).
      onTouchStartCapture: (event: React.TouchEvent) => {
        if (menuOpenRef.current || event.touches.length !== 1) return;
        if (!touchStartRef.current) {
          touchStartRef.current = {
            target: event.target,
            identifier: event.touches[0].identifier,
          };
        }
      },
      onTouchEndCapture: (event: React.TouchEvent) => {
        const tracked = touchStartRef.current;
        if (
          tracked &&
          Array.from(event.changedTouches).some((t) => t.identifier === tracked.identifier)
        ) {
          touchStartRef.current = null;
        }
      },
      onTouchCancelCapture: (event: React.TouchEvent) => {
        const tracked = touchStartRef.current;
        if (
          tracked &&
          Array.from(event.changedTouches).some((t) => t.identifier === tracked.identifier)
        ) {
          touchStartRef.current = null;
        }
      },
    },
  };
}
