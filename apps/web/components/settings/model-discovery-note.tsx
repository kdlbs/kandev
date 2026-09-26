"use client";

import { useTranslation } from "react-i18next";
import { presentModelDiscovery } from "@/lib/agent-host-cli";
import type { ModelDiscovery } from "@/lib/types/http";

/**
 * States the source and version of the profile model list, or the last
 * discovery failure. Renders nothing for an agent type without a host CLI
 * and while the first discovery attempt is still pending.
 */
export function ModelDiscoveryNote({ discovery }: { discovery?: ModelDiscovery }) {
  const { t } = useTranslation();
  const presentation = presentModelDiscovery(discovery);
  if (!presentation) return null;

  let text: string;
  switch (presentation.kind) {
    case "cli":
      text = presentation.version
        ? t("agents:modelDiscoveryFromCli", {
            executable: presentation.executable,
            version: presentation.version,
          })
        : t("agents:modelDiscoveryFromCliNoVersion", { executable: presentation.executable });
      break;
    case "no_listing":
      text = t("agents:modelDiscoveryNoListing", { executable: presentation.executable });
      break;
    case "failed":
      text = t("agents:modelDiscoveryFailed", { reason: presentation.reason });
      break;
  }

  return (
    <p className="text-xs text-muted-foreground" data-testid="model-discovery-note">
      {text}
    </p>
  );
}
