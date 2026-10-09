import { useCallback, useLayoutEffect, useRef, type RefObject } from "react";
import {
  createChatScrollMotion,
  listenForScrollIntent,
  type ScrollMotion,
} from "./chat-scroll-motion";
export type ChatScrollMotionOptions = {
  scrollRef: RefObject<HTMLDivElement | null>;
  motionEnabled: boolean;
  enabled: boolean;
  isVisible: boolean;
  sessionId: string | null;
  isNearBottomRef: RefObject<boolean>;
  isBlocked: () => boolean;
  instant: (element: HTMLElement) => void;
  onUserScrollIntent?: () => void;
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
    if (!element || !options.enabled || !options.isVisible) return;
    const onUserScrollIntent = () => {
      userReading.current = true;
      latest.current.isNearBottomRef.current = false;
      latest.current.onUserScrollIntent?.();
    };
    let motion: ScrollMotion | null = null;
    let removeIntentListener = () => {};
    if (options.motionEnabled) {
      motion = createChatScrollMotion(element, canFollow, onUserScrollIntent);
      driver.current = motion;
    } else {
      removeIntentListener = listenForScrollIntent(element, onUserScrollIntent);
    }
    const observer = new ResizeObserver(() => {
      if (!canFollow()) return;
      if (motion) motion.request();
      else latest.current.instant(element);
    });
    const content = element.querySelector("[data-chat-content]");
    if (content) observer.observe(content);
    return () => {
      const settle = Boolean(motion?.isRunning() && !latest.current.motionEnabled && canFollow());
      observer.disconnect();
      removeIntentListener();
      motion?.dispose();
      if (driver.current === motion) driver.current = null;
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
