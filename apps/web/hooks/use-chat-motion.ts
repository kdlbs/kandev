"use client";

import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";

/** OS motion preferences override the current preview, never its saved value. */
export function useChatMotion(): boolean {
  const enabled = useAppStore((state) => state.chatMotion.enabled);
  const [motionAllowed, setMotionAllowed] = useState(false);
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setMotionAllowed(!media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  return enabled && motionAllowed;
}
