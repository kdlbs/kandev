"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { IconShieldLock } from "@tabler/icons-react";
import { useToast } from "@/components/toast-provider";
import { fetchTelemetryConsent, updateTelemetryConsent } from "@/lib/api/domains/telemetry-api";
import { updateTelemetryConsentCache } from "@/lib/telemetry/track";
import type { TelemetryConsentState } from "@/lib/types/telemetry";

const TELEMETRY_DOCS_URL = "https://github.com/kdlbs/kandev/blob/main/docs/public/telemetry.md";

const COLLECTED_ITEMS = [
  "A daily heartbeat with the Kandev version, OS, CPU architecture, and deploy mode (local, Docker, Kubernetes, or desktop)",
  "Counts of product events: task created/deleted, agent run started/completed/failed (failures carry a coarse error class, never the message), turn completed, workspace created, automation run",
  "Which UI pages and allowlisted actions are used (identifiers only)",
];

const NEVER_COLLECTED_ITEMS = [
  "Prompts, chat messages, code, or diffs",
  "Task titles, repository names, branch names, or file paths",
  "Your name, email, IP-derived profile, or any account identifier",
];

export function TelemetrySettings() {
  const [state, setState] = useState<TelemetryConsentState | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    let cancelled = false;
    fetchTelemetryConsent()
      .then((res) => {
        if (cancelled) return;
        setState(res);
        updateTelemetryConsentCache(res);
      })
      .catch(() => {
        if (!cancelled) setState(null);
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const setConsent = useCallback(
    async (granted: boolean) => {
      setIsSaving(true);
      try {
        const next = await updateTelemetryConsent(granted ? "granted" : "denied");
        setState(next);
        updateTelemetryConsentCache(next);
        toast({
          title: granted ? "Anonymous telemetry enabled" : "Telemetry disabled",
          variant: "success",
        });
      } catch (err) {
        toast({
          title: "Failed to save telemetry preference",
          description: err instanceof Error ? err.message : String(err),
          variant: "error",
        });
      } finally {
        setIsSaving(false);
      }
    },
    [toast],
  );

  if (isLoading) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
          <Spinner className="size-4" />
          Loading telemetry settings...
        </CardContent>
      </Card>
    );
  }

  if (state === null) {
    return (
      <Card>
        <CardContent className="py-6 text-sm text-muted-foreground">
          Telemetry settings could not be loaded.
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4" data-testid="telemetry-settings">
      {state.env_disabled && <EnvDisabledAlert />}
      <ConsentCard state={state} isSaving={isSaving} onConsentChange={setConsent} />
      <CollectedCard />
    </div>
  );
}

function ConsentCard({
  state,
  isSaving,
  onConsentChange,
}: {
  state: TelemetryConsentState;
  isSaving: boolean;
  onConsentChange: (granted: boolean) => Promise<void>;
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
        <div className="space-y-1.5">
          <CardTitle>Share anonymous usage data</CardTitle>
          <CardDescription>
            Strictly opt-in. Helps the Kandev team understand which features matter. Events are
            anonymous, allowlisted, and sent to an EU-hosted PostHog instance.
          </CardDescription>
        </div>
        <Switch
          checked={state.status === "granted"}
          disabled={isSaving || state.env_disabled}
          onCheckedChange={(checked) => void onConsentChange(checked)}
          aria-label="Share anonymous usage data"
          data-testid="telemetry-consent-switch"
        />
      </CardHeader>
      <CardContent className="space-y-2 text-sm text-muted-foreground">
        {state.status === "unasked" && (
          <p>You haven&apos;t decided yet — nothing is shared until you opt in.</p>
        )}
        {state.status === "granted" && state.install_id && (
          <p>
            Anonymous install ID: <span className="font-mono text-xs">{state.install_id}</span>{" "}
            (random UUID, minted when you opted in; deleted if you opt out)
          </p>
        )}
        <p>
          Set <span className="font-mono text-xs">KANDEV_TELEMETRY_DEBUG=1</span> to log every
          outgoing payload locally. The <span className="font-mono text-xs">DO_NOT_TRACK=1</span>{" "}
          convention is honoured and hard-disables collection.
        </p>
      </CardContent>
    </Card>
  );
}

function CollectedCard() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">What is collected</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 text-sm text-muted-foreground">
        <ItemList items={COLLECTED_ITEMS} />
        <div>
          <p className="mb-1 font-medium text-foreground">Never collected</p>
          <ItemList items={NEVER_COLLECTED_ITEMS} />
        </div>
        <p>
          The full event table lives in{" "}
          <a
            href={TELEMETRY_DOCS_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="underline cursor-pointer"
          >
            the telemetry documentation
          </a>
          .
        </p>
      </CardContent>
    </Card>
  );
}

function ItemList({ items }: { items: string[] }) {
  return (
    <ul className="list-disc space-y-1 pl-5">
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  );
}

function EnvDisabledAlert() {
  return (
    <Alert className="border-border/70 bg-muted/30">
      <IconShieldLock className="h-4 w-4 text-muted-foreground" />
      <AlertTitle>Disabled by environment</AlertTitle>
      <AlertDescription className="text-muted-foreground">
        DO_NOT_TRACK is set (or this is a test-mode run), so telemetry is hard-disabled regardless
        of the preference below and no data is ever sent.
      </AlertDescription>
    </Alert>
  );
}
