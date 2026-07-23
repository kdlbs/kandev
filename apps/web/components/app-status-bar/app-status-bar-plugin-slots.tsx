"use client";

import { PluginSlotRegistrationView } from "@/components/plugins/plugin-slot";
import type { PluginSlotRegistration } from "@/lib/plugins/registry";
import type { AppStatusBarSlotProps } from "@/lib/plugins/types";

/** Renders one opaque plugin-owned status item with host-owned context. */
export function AppStatusBarPluginContribution({
  registration,
  ...slotProps
}: AppStatusBarSlotProps & { registration: PluginSlotRegistration }) {
  const name = slotProps.placement === "left" ? "app-status-bar-left" : "app-status-bar-right";
  return (
    <PluginSlotRegistrationView
      key={registration.registrationId}
      registration={registration}
      name={name}
      slotProps={slotProps}
    />
  );
}
