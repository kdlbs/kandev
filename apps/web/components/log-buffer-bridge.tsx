"use client";

import { useEffect } from "react";

import { installConsoleInterceptor } from "@/lib/logger/intercept";
import { setLogIdentity } from "@/lib/logger/runtime";
import { useAppStore } from "@/components/state-provider";

/**
 * Installs the console + window-error interceptor on the client so recent
 * frontend logs are captured for Improve Kandev reports. No UI.
 */
export function LogBufferBridge() {
  const auth = useAppStore((state) => state.auth);
  useEffect(() => {
    installConsoleInterceptor();
  }, []);
  useEffect(() => {
    if (!auth.authenticated) {
      setLogIdentity(null);
      return;
    }
    setLogIdentity(auth.user?.id ?? (auth.mode === "disabled" ? "default-user" : null));
  }, [auth.authenticated, auth.mode, auth.user?.id]);
  return null;
}
