"use client";

import { useAppStore } from "@/components/state-provider";

/** Auth-disabled single-user mode has no user record and retains administrator behavior. */
export function useIsAdmin(): boolean {
  const role = useAppStore((state) => state.auth.user?.role);
  return role === undefined || role === "admin";
}
