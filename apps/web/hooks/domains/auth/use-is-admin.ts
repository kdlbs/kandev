"use client";

import { useAppStore } from "@/components/state-provider";
import { isAdminRole } from "@/lib/auth/is-admin";

/** Whether the current caller may use admin-only, install-wide controls. */
export function useIsAdmin(): boolean {
  const role = useAppStore((state) => state.auth.user?.role);
  return isAdminRole(role);
}
