"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { IconChartDots, IconCheck, IconShieldLock } from "@tabler/icons-react";
import { fetchTelemetryConsent, updateTelemetryConsent } from "@/lib/api/domains/telemetry-api";
import { updateTelemetryConsentCache } from "@/lib/telemetry/track";
import type { TelemetryConsentState, TelemetryConsentStatus } from "@/lib/types/telemetry";

/**
 * Decides whether the onboarding dialog should show the telemetry consent
 * step: only when the install has never been asked and the env kill
 * switches (KANDEV_TELEMETRY=off / DO_NOT_TRACK) are not active.
 */
export function useTelemetryOnboarding(open: boolean): { showTelemetryStep: boolean } {
  const [consent, setConsent] = useState<TelemetryConsentState | null>(null);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    fetchTelemetryConsent()
      .then((res) => {
        if (cancelled) return;
        setConsent(res);
        updateTelemetryConsentCache(res);
      })
      .catch(() => {
        // Backend without telemetry support (or transient error): no step.
      });
    return () => {
      cancelled = true;
    };
  }, [open]);

  return {
    showTelemetryStep: consent !== null && consent.status === "unasked" && !consent.env_disabled,
  };
}

const SHARED_ITEMS = [
  "Version, OS, and deploy mode (daily heartbeat)",
  "Counts of events like task created or agent run completed",
  "Which UI pages and features are used (identifiers only)",
];

const NEVER_ITEMS = ["Prompts, code, or diffs", "Task titles, repo names, branches, or paths"];

/**
 * The consent step body. Choosing either option records the decision
 * immediately; leaving the step untouched keeps telemetry off and the
 * question available later under Settings → System → Telemetry.
 */
export function StepTelemetry() {
  const [choice, setChoice] = useState<Exclude<TelemetryConsentStatus, "unasked"> | null>(null);
  const [isSaving, setIsSaving] = useState(false);

  const choose = async (status: Exclude<TelemetryConsentStatus, "unasked">) => {
    setIsSaving(true);
    try {
      const next = await updateTelemetryConsent(status);
      updateTelemetryConsentCache(next);
      setChoice(status);
    } catch {
      // Leave undecided; the settings page remains available.
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="grid gap-2 sm:grid-cols-2">
        <div className="rounded-lg border p-3">
          <div className="flex items-center gap-2 text-sm font-medium">
            <IconChartDots className="h-4 w-4 text-muted-foreground" />
            Shared if you opt in
          </div>
          <ItemList items={SHARED_ITEMS} />
        </div>
        <div className="rounded-lg border p-3">
          <div className="flex items-center gap-2 text-sm font-medium">
            <IconShieldLock className="h-4 w-4 text-muted-foreground" />
            Never shared
          </div>
          <ItemList items={NEVER_ITEMS} />
        </div>
      </div>
      {choice === null ? (
        <div className="flex flex-col items-center gap-2 sm:flex-row sm:justify-center">
          <Button
            onClick={() => void choose("granted")}
            disabled={isSaving}
            className="cursor-pointer"
            data-testid="onboarding-telemetry-accept"
          >
            Share anonymous usage data
          </Button>
          <Button
            variant="outline"
            onClick={() => void choose("denied")}
            disabled={isSaving}
            className="cursor-pointer"
            data-testid="onboarding-telemetry-decline"
          >
            No thanks
          </Button>
        </div>
      ) : (
        <p className="flex items-center justify-center gap-1.5 text-sm text-muted-foreground">
          <IconCheck className="h-4 w-4 text-green-600 dark:text-green-400" />
          {choice === "granted"
            ? "Thank you! Anonymous usage sharing is on."
            : "Understood — nothing will be shared."}{" "}
          Change this anytime in Settings → System → Telemetry.
        </p>
      )}
      <p className="text-center text-xs text-muted-foreground">
        Strictly opt-in and anonymous (random install ID, EU-hosted). Skipping this step keeps
        sharing off.
      </p>
    </div>
  );
}

function ItemList({ items }: { items: string[] }) {
  return (
    <ul className="mt-1.5 list-disc space-y-1 pl-5 text-xs text-muted-foreground">
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  );
}
