"use client";

import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  resolveLspStatusPlacement,
  type EffectiveLspStatusPlacement,
} from "@/lib/lsp/lsp-status-placement";

export function useLspStatusPlacement(): EffectiveLspStatusPlacement {
  const preferredLocation = useAppStore((state) => state.userSettings.lspStatusLocation);
  const appStatusBarEnabled = useAppStore((state) => state.userSettings.appStatusBarEnabled);
  const responsive = useResponsiveBreakpoint();
  return resolveLspStatusPlacement({
    preferredLocation,
    appStatusBarEnabled,
    hasFinePointer: responsive.isFinePointer,
    isPhone: responsive.isMobile,
  });
}
