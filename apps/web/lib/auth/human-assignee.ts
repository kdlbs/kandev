import type { AuthSliceState } from "@/lib/state/slices/auth/types";

export function canShowHumanAssignee(auth: AuthSliceState["auth"]): boolean {
  return auth.mode === "enabled" && auth.user !== null;
}
