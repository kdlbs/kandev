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
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle, IconPower, IconRotateClockwise } from "@tabler/icons-react";
import { useToast } from "@/components/toast-provider";
import { useKandevRestart } from "@/hooks/domains/system/use-kandev-restart";
import { fetchRuntimeFlags, updateRuntimeFlag } from "@/lib/api/domains/runtime-flags-api";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import type { RestartCapability } from "@/lib/types/system";
import { FeatureToggleCard } from "./feature-toggle-card";
import { RestartProgressDialog } from "./restart-progress-dialog";

type Props = {
  initialFlags: RuntimeFlagState[];
  restartCapability: RestartCapability | null;
};

export function FeatureTogglesSettings({ initialFlags, restartCapability }: Props) {
  const [flags, setFlags] = useState(initialFlags);
  const [savingKeys, setSavingKeys] = useState<Set<string>>(() => new Set());
  const requestSeqRef = useRef(0);
  const { toast } = useToast();
  const restart = useKandevRestart({ onComplete: () => void reload() });
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

  return (
    <div className="space-y-4" data-testid="feature-toggles-settings">
      {pendingRestart && (
        <RestartRequiredAlert
          capability={restartCapability}
          restarting={restart.isRestarting}
          onRestart={() => void restart.start()}
        />
      )}
      {flags.map((flag) => (
        <FeatureToggleCard
          key={flag.key}
          flag={flag}
          saving={savingKeys.has(flag.key) || restart.isRestarting}
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
      <RestartProgressDialog
        phase={restart.phase}
        errorMessage={restart.errorMessage}
        onDismiss={restart.dismiss}
      />
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
    <Alert className="border-border/70 bg-muted/30">
      <IconRotateClockwise className="h-4 w-4 text-muted-foreground" />
      <AlertTitle className="flex items-center gap-2">
        Restart required
        <RestartSupportInfo supported={supported} reason={capability?.reason} />
      </AlertTitle>
      <AlertDescription className="flex flex-col gap-3 text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
        <span>
          Saved toggle changes will apply the next time Kandev starts.
          {!supported && " Restart it from your terminal or service manager when convenient."}
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

function RestartSupportInfo({
  supported,
  reason,
}: {
  supported: boolean;
  reason: string | undefined;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label="Restart support details"
          className="inline-flex h-6 w-6 cursor-help items-center justify-center rounded-md text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <IconInfoCircle className="h-4 w-4" />
        </button>
      </TooltipTrigger>
      <TooltipContent side="right" className="max-w-xs text-xs leading-relaxed">
        {restartSupportMessage(supported, reason)}
      </TooltipContent>
    </Tooltip>
  );
}

function restartSupportMessage(supported: boolean, reason: string | undefined): string {
  if (supported) {
    return "Restart from this page is available when Kandev is running under a supported local supervisor.";
  }
  return (
    reason ??
    "Automatic restart is not available in deploy previews, unmanaged terminal runs, or launch modes without a restart supervisor."
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
