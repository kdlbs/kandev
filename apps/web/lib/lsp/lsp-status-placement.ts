import type { LspStatusLocation } from "@/lib/types/http";

export type EffectiveLspStatusPlacement = LspStatusLocation | "hidden";

type ResolveLspStatusPlacementInput = {
  preferredLocation: LspStatusLocation | string | undefined;
  appStatusBarEnabled: boolean;
  hasFinePointer: boolean;
  isPhone: boolean;
};

export function resolveLspStatusPlacement({
  preferredLocation,
  appStatusBarEnabled,
  hasFinePointer,
  isPhone,
}: ResolveLspStatusPlacementInput): EffectiveLspStatusPlacement {
  if (isPhone) return "hidden";
  if (preferredLocation === "status_bar" && appStatusBarEnabled && hasFinePointer) {
    return "status_bar";
  }
  return "toolbar";
}
