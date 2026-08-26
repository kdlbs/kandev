"use client";

import { useAppStore } from "@/components/state-provider";
import { isAdminIdentity } from "@/lib/auth/is-admin";

/** Whether the current caller may use admin-only, install-wide controls. */
export function useIsAdmin(): boolean {
  const mode = useAppStore((state) => state.auth.mode);
  const role = useAppStore((state) => state.auth.user?.role);
  return isAdminIdentity(mode, role);
}
