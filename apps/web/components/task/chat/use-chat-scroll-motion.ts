import { useCallback, useLayoutEffect, useRef, type RefObject } from "react";
import { createChatScrollMotion, type ScrollMotion } from "./chat-scroll-motion";
export type ChatScrollMotionOptions = {
  scrollRef: RefObject<HTMLDivElement | null>;
  motionEnabled: boolean;
  enabled: boolean;
  isVisible: boolean;
  sessionId: string | null;
  isNearBottomRef: RefObject<boolean>;
  isBlocked: () => boolean;
  instant: (element: HTMLElement) => void;
};
export function useChatScrollMotion(options: ChatScrollMotionOptions) {
  const latest = useRef(options);
  latest.current = options;
  const driver = useRef<ScrollMotion | null>(null);
  const userReading = useRef(false);
  const canFollow = useCallback(() => {
    const value = latest.current;
    return value.enabled && value.isVisible && !value.isBlocked() && value.isNearBottomRef.current;
  }, []);
  useLayoutEffect(() => {
    const element = options.scrollRef.current;
    if (!element || !options.motionEnabled || !options.enabled || !options.isVisible) return;
    const motion = createChatScrollMotion(element, canFollow, () => {
      userReading.current = true;
      latest.current.isNearBottomRef.current = false;
    });
    driver.current = motion;
    const observer = new ResizeObserver(() => {
      if (canFollow()) motion.request();
    });
    const content = element.querySelector("[data-chat-content]");
    if (content) observer.observe(content);
    return () => {
      const settle = motion.isRunning() && !latest.current.motionEnabled && canFollow();
      observer.disconnect();
      motion.dispose();
      driver.current = null;
      if (settle) latest.current.instant(element);
    };
  }, [
    options.scrollRef,
    options.motionEnabled,
    options.enabled,
    options.isVisible,
    options.sessionId,
    canFollow,
  ]);
  const followBottom = useCallback(() => {
    const value = latest.current;
    const element = value.scrollRef.current;
    if (!element || !value.enabled || !value.isVisible || value.isBlocked()) return;
    userReading.current = false;
    value.isNearBottomRef.current = true;
    if (driver.current) driver.current.request();
    else value.instant(element);
  }, []);
  const cancel = useCallback(() => driver.current?.cancel(), []);
  const isAnimating = useCallback(() => driver.current?.isRunning() ?? false, []);
  return { followBottom, cancel, isAnimating, userReading };
}
