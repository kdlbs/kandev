"use client";

import { useState } from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@kandev/ui/alert-dialog";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { IconDownload, IconExternalLink, IconRefresh } from "@tabler/icons-react";
import { useSelfUpdate } from "@/hooks/domains/system/use-self-update";
import {
  useDesktopUpdater,
  type DesktopUpdaterController,
} from "@/hooks/domains/system/use-desktop-updater";
import { useUpdates } from "@/hooks/domains/system/use-updates";
import type { UpdatesResponse } from "@/lib/types/system";
import { SelfUpdateProgress } from "./self-update-progress";

interface ApplyGate {
  canApply: boolean;
  cannotApplyReason?: string;
  manualCommands: string[];
}

function formatChecked(value: string | number | null | undefined): string {
  if (!value) return "never";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return String(value);
  return d.toLocaleString();
}

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

function retryAfterSeconds(message: string): number | null {
  const match = /retry.*?(\d+)/i.exec(message);
  return match ? Number(match[1]) : null;
}

function getApplyGate(updates: UpdatesResponse | null | undefined): ApplyGate {
  const install = updates?.install;
  const available = updates?.update_available === true;
  const canApply =
    available &&
    install?.running_as_service === true &&
    install.managed_service === true &&
    updates?.apply_supported === true;
  return {
    canApply,
    cannotApplyReason: updates?.apply_unsupported_reason,
    manualCommands: updates?.manual_commands ?? [],
  };
}

export function UpdatesCard() {
  const { updates, check, reload } = useUpdates();
  const selfUpdate = useSelfUpdate({ latestVersion: updates?.latest, onComplete: reload });
  const desktopUpdater = useDesktopUpdater();
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [retryAfter, setRetryAfter] = useState<number | null>(null);

  if (desktopUpdater.available) {
    return <DesktopUpdatesCard updater={desktopUpdater} />;
  }

  const onCheck = async () => {
    setChecking(true);
    setError(null);
    setRetryAfter(null);
    try {
      await check();
    } catch (err) {
      const message = errorMessage(err, "Update check failed");
      setError(message);
      setRetryAfter(retryAfterSeconds(message));
    } finally {
      setChecking(false);
    }
  };

  const current = updates?.current ?? "-";
  const latest = updates?.latest ?? "-";
  const available = updates?.update_available === true;
  const gate = getApplyGate(updates);
  // Hide the Apply button while an update is in flight (and once it's done —
  // the version has flipped) so it can't be re-triggered mid-restart.
  const showApply = gate.canApply && !selfUpdate.isUpdating && selfUpdate.phase !== "done";

  return (
    <Card data-testid="system-updates-card">
      <CardHeader>
        <UpdatesHeader available={available} />
      </CardHeader>
      <CardContent className="space-y-4">
        <VersionGrid current={current} latest={latest} />
        <LastChecked checkedAt={updates?.latest_checked_at} />
        <UpdateActions
          checking={checking}
          showApply={showApply}
          latest={latest}
          url={updates?.latest_url}
          onCheck={onCheck}
          onApply={selfUpdate.start}
        />
        <ManualUpdateInstructions
          show={available && !gate.canApply && !selfUpdate.isUpdating}
          reason={gate.cannotApplyReason}
          commands={gate.manualCommands}
        />
        <SelfUpdateProgress
          phase={selfUpdate.phase}
          targetVersion={selfUpdate.targetVersion}
          errorMessage={selfUpdate.errorMessage}
          onDismiss={selfUpdate.dismiss}
        />
        <UpdateError error={error} retryAfter={retryAfter} />
      </CardContent>
    </Card>
  );
}

function DesktopUpdatesCard({ updater }: { updater: DesktopUpdaterController }) {
  const view = desktopCardView(updater);

  return (
    <Card data-testid="system-updates-card">
      <CardHeader>
        <UpdatesHeader available={view.available} />
      </CardHeader>
      <CardContent className="space-y-4">
        <VersionGrid current={view.current} latest={view.latest} />
        <LastChecked checkedAt={updater.state?.checkedAtEpochMs} />
        <UpdateActions
          checking={view.checking}
          showApply={view.showApply}
          latest={view.latest}
          url={updater.state?.releaseUrl ?? undefined}
          onCheck={() => ignoreFailure(updater.check())}
          onApply={() => ignoreFailure(updater.install())}
          desktop
        />
        <ManualUpdateInstructions
          show={view.available && !view.installSupported}
          reason={updater.state?.installUnsupportedReason ?? undefined}
          commands={[]}
        />
        <DesktopCurrentStatus phase={updater.state?.phase} />
        <DesktopUpdateProgress updater={updater} />
        <UpdateError error={updater.error} retryAfter={null} />
      </CardContent>
    </Card>
  );
}

function desktopCardView(updater: DesktopUpdaterController) {
  const state = updater.state;
  if (!state) {
    return {
      available: false,
      checking: updater.checking,
      showApply: false,
      installSupported: false,
      current: "-",
      latest: "-",
    };
  }
  const available = state.phase === "available";
  const installing = updater.installing || ["downloading", "installing"].includes(state.phase);
  return {
    available,
    checking: updater.checking || state.phase === "checking",
    showApply: available && state.installSupported === true && !installing,
    installSupported: state.installSupported === true,
    current: state.currentVersion,
    latest: state.latestVersion ?? (state.phase === "up-to-date" ? state.currentVersion : "-"),
  };
}

function DesktopCurrentStatus({ phase }: { phase: string | undefined }) {
  if (phase !== "up-to-date") return null;
  return (
    <p className="text-xs text-muted-foreground" data-testid="system-updates-current-status">
      Kandev is up to date.
    </p>
  );
}

async function ignoreFailure(operation: Promise<void>): Promise<void> {
  await operation.catch(() => undefined);
}

function DesktopUpdateProgress({ updater }: { updater: DesktopUpdaterController }) {
  const state = updater.state;
  if (state?.phase !== "downloading" && state?.phase !== "installing") return null;
  let detail = "Installing update...";
  if (state.phase === "downloading") {
    const downloaded = state.downloadedBytes ?? 0;
    detail = state.totalBytes
      ? `Downloading update (${downloaded} of ${state.totalBytes} bytes)...`
      : `Downloading update (${downloaded} bytes)...`;
  }
  return (
    <div
      className="flex items-center gap-2 text-xs text-muted-foreground"
      data-testid="system-updates-progress"
      data-phase={state.phase}
    >
      <Spinner className="size-3.5" />
      {detail}
    </div>
  );
}

function UpdatesHeader({ available }: { available: boolean }) {
  return (
    <CardTitle className="text-base flex items-center gap-2">
      <IconRefresh className="h-4 w-4" />
      Updates
      {available && (
        <Badge variant="default" className="text-[10px]" data-testid="system-updates-badge">
          Update available
        </Badge>
      )}
    </CardTitle>
  );
}

function VersionGrid({ current, latest }: { current: string; latest: string }) {
  return (
    <div className="grid grid-cols-2 gap-3 text-sm">
      <VersionValue label="Current version" value={current} testId="system-updates-current" />
      <VersionValue label="Latest release" value={latest} testId="system-updates-latest" />
    </div>
  );
}

function VersionValue({ label, value, testId }: { label: string; value: string; testId: string }) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="font-mono text-sm" data-testid={testId}>
        {value}
      </div>
    </div>
  );
}

function LastChecked({ checkedAt }: { checkedAt?: string | number | null }) {
  return (
    <div className="text-xs text-muted-foreground" data-testid="system-updates-checked-at">
      Last checked {formatChecked(checkedAt)}
    </div>
  );
}

interface UpdateActionsProps {
  checking: boolean;
  showApply: boolean;
  latest: string;
  url?: string;
  onCheck: () => Promise<void>;
  onApply: () => Promise<void>;
  desktop?: boolean;
}

function UpdateActions(props: UpdateActionsProps) {
  return (
    <div
      className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center"
      data-testid="system-updates-actions"
    >
      <CheckNowButton checking={props.checking} onCheck={props.onCheck} />
      <ReleaseNotesLink url={props.url} />
      <ApplyUpdateDialog
        showApply={props.showApply}
        latest={props.latest}
        onApply={props.onApply}
        desktop={props.desktop}
      />
    </div>
  );
}

function CheckNowButton({
  checking,
  onCheck,
}: {
  checking: boolean;
  onCheck: () => Promise<void>;
}) {
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={checking}
      onClick={() => void onCheck()}
      className="cursor-pointer"
      data-testid="system-updates-check"
    >
      {checking ? (
        <Spinner className="size-3.5 mr-1" />
      ) : (
        <IconRefresh className="h-3.5 w-3.5 mr-1" />
      )}
      Check now
    </Button>
  );
}

function ReleaseNotesLink({ url }: { url?: string }) {
  if (!url) return null;
  return (
    <Button
      asChild
      variant="ghost"
      size="sm"
      className="cursor-pointer"
      data-testid="system-updates-release-link"
    >
      <a href={url} target="_blank" rel="noreferrer">
        Release notes
        <IconExternalLink className="h-3.5 w-3.5 ml-1" />
      </a>
    </Button>
  );
}

interface ApplyUpdateDialogProps {
  showApply: boolean;
  latest: string;
  onApply: () => Promise<void>;
  desktop?: boolean;
}

function ApplyUpdateDialog({ showApply, latest, onApply, desktop }: ApplyUpdateDialogProps) {
  if (!showApply) return null;
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" className="cursor-pointer" data-testid="system-updates-apply">
          <IconDownload className="h-3.5 w-3.5 mr-1" />
          Apply update
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Apply update?</AlertDialogTitle>
          <AlertDialogDescription className="text-left">
            {desktop
              ? `Kandev will install ${latest} and restart the desktop app.`
              : `Kandev will update to ${latest}, reinstall the user service, and restart it.`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer"
            onClick={() => void onApply()}
            data-testid="system-updates-apply-confirm"
          >
            Apply update
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function ManualUpdateInstructions({
  show,
  reason,
  commands,
}: {
  show: boolean;
  reason?: string;
  commands: string[];
}) {
  if (!show || !reason) return null;
  return (
    <div className="space-y-2 text-xs text-muted-foreground" data-testid="system-updates-manual">
      <p>{reason}</p>
      <ManualCommands commands={commands} />
    </div>
  );
}

function ManualCommands({ commands }: { commands: string[] }) {
  if (commands.length === 0) return null;
  return (
    <div className="space-y-1">
      {commands.map((cmd) => (
        <code key={cmd} className="block break-all rounded bg-muted px-2 py-1 font-mono">
          {cmd}
        </code>
      ))}
    </div>
  );
}

function UpdateError({ error, retryAfter }: { error: string | null; retryAfter: number | null }) {
  if (!error) return null;
  return (
    <p className="text-xs text-destructive" data-testid="system-updates-error">
      {retryAfter ? `Already checked. Try again in ${retryAfter}s.` : error}
    </p>
  );
}
