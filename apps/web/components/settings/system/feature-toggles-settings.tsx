"use client";

import {
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from "react";
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
  const [savingKeys, setSavingKeys] = useState<Set<string>>(() => new Set());
  const [restarting, setRestarting] = useState(false);
  const requestSeqRef = useRef(0);
  const { toast } = useToast();
  const pendingRestart = useMemo(
    () => flags.some((flag) => flag.requires_restart_to_apply),
    [flags],
  );

  const reload = async () => {
    const seq = nextRequestSeq(requestSeqRef);
    try {
      const res = await fetchRuntimeFlags();
      setFlagsIfLatest(requestSeqRef, seq, res.flags, setFlags);
    } catch (err) {
      toast({
        title: "Failed to load feature toggles",
        description: errorMessage(err),
        variant: "error",
      });
    }
  };

  const setOverride = async (flag: RuntimeFlagState, override: boolean | null) => {
    const seq = nextRequestSeq(requestSeqRef);
    setSavingKeys((prev) => {
      const next = new Set(prev);
      next.add(flag.key);
      return next;
    });
    try {
      const res = await updateRuntimeFlag(flag.key, override);
      setFlagsIfLatest(requestSeqRef, seq, res.flags, setFlags);
      toast({ title: "Feature toggle saved", variant: "success" });
    } catch (err) {
      toast({
        title: "Failed to save feature toggle",
        description: errorMessage(err),
        variant: "error",
      });
    } finally {
      setSavingKeys((prev) => {
        const next = new Set(prev);
        next.delete(flag.key);
        return next;
      });
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
          saving={savingKeys.has(flag.key)}
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

function nextRequestSeq(seqRef: MutableRefObject<number>): number {
  seqRef.current += 1;
  return seqRef.current;
}

function setFlagsIfLatest(
  seqRef: MutableRefObject<number>,
  seq: number,
  flags: RuntimeFlagState[],
  setFlags: Dispatch<SetStateAction<RuntimeFlagState[]>>,
) {
  if (seq === seqRef.current) {
    setFlags(flags);
  }
}
