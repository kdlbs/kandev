"use client";

import { useMemo, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { IconPower, IconRotateClockwise } from "@tabler/icons-react";
import { useToast } from "@/components/toast-provider";
import { fetchRuntimeFlags, updateRuntimeFlag } from "@/lib/api/domains/runtime-flags-api";
import { requestRestart } from "@/lib/api/domains/system-api";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import type { RestartCapability } from "@/lib/types/system";
import { FeatureToggleCard } from "./feature-toggle-card";

type Props = {
  initialFlags: RuntimeFlagState[];
  restartCapability: RestartCapability | null;
};

export function FeatureTogglesSettings({ initialFlags, restartCapability }: Props) {
  const [flags, setFlags] = useState(initialFlags);
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [restarting, setRestarting] = useState(false);
  const { toast } = useToast();
  const pendingRestart = useMemo(
    () => flags.some((flag) => flag.requires_restart_to_apply),
    [flags],
  );

  const reload = async () => {
    const res = await fetchRuntimeFlags();
    setFlags(res.flags);
  };

  const setOverride = async (flag: RuntimeFlagState, override: boolean | null) => {
    setSavingKey(flag.key);
    try {
      const res = await updateRuntimeFlag(flag.key, override);
      setFlags(res.flags);
      toast({ title: "Feature toggle saved", variant: "success" });
    } catch (err) {
      toast({
        title: "Failed to save feature toggle",
        description: errorMessage(err),
        variant: "error",
      });
    } finally {
      setSavingKey(null);
    }
  };

  const restart = async () => {
    setRestarting(true);
    try {
      const res = await requestRestart();
      toast({ title: "Restart requested", description: res.message, variant: "success" });
    } catch (err) {
      toast({ title: "Restart unavailable", description: errorMessage(err), variant: "error" });
    } finally {
      setRestarting(false);
    }
  };

  return (
    <div className="space-y-4" data-testid="feature-toggles-settings">
      {pendingRestart && (
        <RestartRequiredAlert
          capability={restartCapability}
          restarting={restarting}
          onRestart={() => void restart()}
        />
      )}
      {flags.map((flag) => (
        <FeatureToggleCard
          key={flag.key}
          flag={flag}
          saving={savingKey === flag.key}
          onChange={(next) => void setOverride(flag, next)}
          onReset={() => void setOverride(flag, null)}
        />
      ))}
      {flags.length === 0 && (
        <Card>
          <CardContent className="py-6 text-sm text-muted-foreground">
            Feature toggles could not be loaded.
            <Button
              variant="link"
              className="h-auto px-1 cursor-pointer"
              onClick={() => void reload()}
            >
              Retry
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function RestartRequiredAlert({
  capability,
  restarting,
  onRestart,
}: {
  capability: RestartCapability | null;
  restarting: boolean;
  onRestart: () => void;
}) {
  const supported = capability?.supported === true;
  return (
    <Alert>
      <IconRotateClockwise className="h-4 w-4" />
      <AlertTitle>Restart required</AlertTitle>
      <AlertDescription className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <span>
          Saved toggle changes will apply after Kandev restarts.
          {!supported &&
            ` ${capability?.reason ?? "Restart Kandev from the terminal or service manager."}`}
        </span>
        {supported && (
          <Button
            size="sm"
            onClick={onRestart}
            disabled={restarting}
            className="w-full cursor-pointer sm:w-auto"
          >
            <IconPower className="mr-1 h-3.5 w-3.5" />
            Restart
          </Button>
        )}
      </AlertDescription>
    </Alert>
  );
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
