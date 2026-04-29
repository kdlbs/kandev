"use client";

import { useEffect } from "react";

import { installConsoleInterceptor } from "@/lib/logger/intercept";

/**
 * Installs the console + window-error interceptor on the client so recent
 * frontend logs are captured for Improve Kandev reports. No UI.
 */
export function LogBufferBridge() {
  useEffect(() => {
    installConsoleInterceptor();
  }, []);
  return null;
}
