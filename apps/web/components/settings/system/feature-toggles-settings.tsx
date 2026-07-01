"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconInfoCircle, IconPower, IconRotateClockwise } from "@tabler/icons-react";
import { useToast } from "@/components/toast-provider";
import { useKandevRestart } from "@/hooks/domains/system/use-kandev-restart";
import { updateRuntimeFlag } from "@/lib/api/domains/runtime-flags-api";
import { qk } from "@/lib/query/keys";
import { runtimeFlagsQueryOptions } from "@/lib/query/query-options/settings";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import type { RestartCapability } from "@/lib/types/system";
import { FeatureToggleCard } from "./feature-toggle-card";
import { RestartProgressDialog } from "./restart-progress-dialog";
import { useSettingsSaveContributor } from "../settings-save-provider";

type Props = {
  initialFlags: RuntimeFlagState[];
  restartCapability: RestartCapability | null;
};

export function FeatureTogglesSettings({ initialFlags, restartCapability }: Props) {
  const { flags, savedFlags, isLoadingFlags, isDirty, reload, setOverride } =
    useRuntimeFlagsDraft(initialFlags);
  const pendingRestart = useMemo(
    () => flags.some((flag) => flag.requires_restart_to_apply),
    [flags],
  );
  const savedFlagsByKey = useMemo(
    () => new Map(savedFlags.map((flag) => [flag.key, flag])),
    [savedFlags],
  );
  const onRestartComplete = useCallback(() => void reload(), [reload]);
  const restart = useKandevRestart({ onComplete: onRestartComplete });

  return (
    <div className="space-y-4" data-testid="feature-toggles-settings">
      {pendingRestart && (
        <RestartRequiredAlert
          capability={restartCapability}
          restarting={restart.isRestarting || isDirty}
          onRestart={() => void restart.start()}
        />
      )}
      {flags.map((flag) => (
        <FeatureToggleCard
          key={flag.key}
          flag={flag}
          isDirty={flag.override_value !== savedFlagsByKey.get(flag.key)?.override_value}
          saving={restart.isRestarting}
          onChange={(next) => setOverride(flag, next)}
          onReset={() => setOverride(flag, null)}
        />
      ))}
      {flags.length === 0 && (
        <FeatureTogglesEmptyState isLoading={isLoadingFlags} onRetry={() => void reload()} />
      )}
      <RestartProgressDialog
        phase={restart.phase}
        errorMessage={restart.errorMessage}
        onDismiss={restart.dismiss}
      />
    </div>
  );
}

function useRuntimeFlagsDraft(initialFlags: RuntimeFlagState[]) {
  const queryClient = useQueryClient();
  const flagsQuery = useQuery({
    ...runtimeFlagsQueryOptions(),
    initialData: initialFlags.length > 0 ? { flags: initialFlags } : undefined,
  });
  const [flags, setFlags] = useState(initialFlags);
  const [savedFlags, setSavedFlags] = useState(initialFlags);
  const savedFlagsRef = useRef(savedFlags);
  savedFlagsRef.current = savedFlags;
  const { toast } = useToast();

  const reload = useCallback(async () => {
    await flagsQuery.refetch();
  }, [flagsQuery, toast]);

  useEffect(() => {
    if (!flagsQuery.isSuccess) return;
    const next = flagsQuery.data.flags;
    const savedRevision = flagsRevision(savedFlagsRef.current);
    setFlags((current) => (flagsRevision(current) === savedRevision ? next : current));
    setSavedFlags(next);
  }, [flagsQuery.data, flagsQuery.isSuccess]);

  useEffect(() => {
    if (!flagsQuery.error) return;
    toast({
      title: "Failed to load feature toggles",
      description: errorMessage(flagsQuery.error),
      variant: "error",
    });
  }, [flagsQuery.error, toast]);

  const setOverride = (flag: RuntimeFlagState, override: boolean | null) => {
    setFlags((current) =>
      current.map((item) =>
        item.key === flag.key
          ? {
              ...item,
              override_value: override,
              effective_value: override ?? item.default_value,
              source: override === null ? "default" : "override",
            }
          : item,
      ),
    );
  };

  const revision = flagsRevision(flags);
  const savedRevision = flagsRevision(savedFlags);
  const isDirty = revision !== savedRevision;
  useSettingsSaveContributor({
    id: "system-feature-toggles",
    revision,
    isDirty,
    save: async () => {
      const submitted = flags;
      const changed = submitted.filter((flag) => {
        const saved = savedFlags.find((candidate) => candidate.key === flag.key);
        return saved?.override_value !== flag.override_value;
      });
      let persisted = savedFlags;
      for (const flag of changed) {
        const response = await updateRuntimeFlag(flag.key, flag.override_value);
        persisted = response.flags;
      }
      queryClient.setQueryData(qk.settings.runtimeFlags(), { flags: persisted });
      setSavedFlags(persisted);
      setFlags((current) => (current === submitted ? persisted : current));
      toast({ title: "Feature toggles saved", variant: "success" });
    },
    discard: () => setFlags(savedFlags),
  });

  return {
    flags,
    savedFlags,
    isLoadingFlags: flagsQuery.isFetching && flags.length === 0,
    isDirty,
    reload,
    setOverride,
  };
}

function flagsRevision(flags: RuntimeFlagState[]): string {
  return JSON.stringify(flags.map(({ key, override_value }) => [key, override_value]));
}

function FeatureTogglesEmptyState({
  isLoading,
  onRetry,
}: {
  isLoading: boolean;
  onRetry: () => void;
}) {
  if (isLoading) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
          <Spinner className="size-4" />
          Loading feature toggles...
        </CardContent>
      </Card>
    );
  }
  return (
    <Card>
      <CardContent className="py-6 text-sm text-muted-foreground">
        Feature toggles could not be loaded.
        <Button variant="link" className="h-auto px-1 cursor-pointer" onClick={onRetry}>
          Retry
        </Button>
      </CardContent>
    </Card>
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
