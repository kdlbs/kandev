type ScrollTarget = {
  isConnected: boolean;
  scrollTop: number;
};

type ScheduleClampedScrollRestoreOptions = {
  element: ScrollTarget;
  targetScrollTop: number;
  onApply: () => void;
  maxFrames?: number;
  requestFrame?: (callback: FrameRequestCallback) => number;
};

/**
 * Re-applies a saved offset while late layout growth is still clamping it.
 * The work is frame-bounded and stops immediately if Dockview removes the
 * target from the document, so a remount retry cannot keep mutating detached
 * DOM.
 */
export function scheduleClampedScrollRestore({
  element,
  targetScrollTop,
  onApply,
  maxFrames = 20,
  requestFrame = requestAnimationFrame,
}: ScheduleClampedScrollRestoreOptions): void {
  let framesRemaining = maxFrames;
  const reapply = () => {
    if (!element.isConnected || framesRemaining <= 0) return;
    framesRemaining -= 1;
    element.scrollTop = targetScrollTop;
    onApply();
    if (element.scrollTop < targetScrollTop - 1) {
      requestFrame(reapply);
    }
  };
  requestFrame(reapply);
}
