"use client";

import { SandboxSelectionSettings } from "./sandbox-selection-settings";

export function AgentSandboxSettings({ profileId }: { profileId: string }) {
  return <SandboxSelectionSettings scope="agent" targetId={profileId} />;
}
