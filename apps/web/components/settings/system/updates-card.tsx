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
import { useUpdates } from "@/hooks/domains/system/use-updates";
import type { UpdatesResponse } from "@/lib/types/system";
import { JobProgressIndicator } from "./job-progress-indicator";

interface ApplyGate {
  canApply: boolean;
  cannotApplyReason?: string;
  manualCommands: string[];
}

function formatChecked(iso: string | undefined): string {
  if (!iso) return "never";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
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
  const { updates, check, apply, isApplying } = useUpdates();
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [retryAfter, setRetryAfter] = useState<number | null>(null);
  const [applyJobId, setApplyJobId] = useState<string | null>(null);

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

  const onApply = async () => {
    setError(null);
    try {
      const res = await apply();
      setApplyJobId(res.job_id);
    } catch (err) {
      setError(errorMessage(err, "Update apply failed"));
    }
  };

  const current = updates?.current ?? "-";
  const latest = updates?.latest ?? "-";
  const available = updates?.update_available === true;
  const gate = getApplyGate(updates);

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
          isApplying={isApplying}
          canApply={gate.canApply}
          latest={latest}
          url={updates?.latest_url}
          onCheck={onCheck}
          onApply={onApply}
        />
        <ManualUpdateInstructions
          show={available && !gate.canApply}
          reason={gate.cannotApplyReason}
          commands={gate.manualCommands}
        />
        <JobProgressIndicator kind="self-update" jobId={applyJobId ?? undefined} />
        <UpdateError error={error} retryAfter={retryAfter} />
      </CardContent>
    </Card>
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

function LastChecked({ checkedAt }: { checkedAt?: string }) {
  return (
    <div className="text-xs text-muted-foreground" data-testid="system-updates-checked-at">
      Last checked {formatChecked(checkedAt)}
    </div>
  );
}

interface UpdateActionsProps {
  checking: boolean;
  isApplying: boolean;
  canApply: boolean;
  latest: string;
  url?: string;
  onCheck: () => Promise<void>;
  onApply: () => Promise<void>;
}

function UpdateActions(props: UpdateActionsProps) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
      <CheckNowButton checking={props.checking} onCheck={props.onCheck} />
      <ReleaseNotesLink url={props.url} />
      <ApplyUpdateDialog
        canApply={props.canApply}
        latest={props.latest}
        isApplying={props.isApplying}
        onApply={props.onApply}
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
  canApply: boolean;
  latest: string;
  isApplying: boolean;
  onApply: () => Promise<void>;
}

function ApplyUpdateDialog({ canApply, latest, isApplying, onApply }: ApplyUpdateDialogProps) {
  if (!canApply) return null;
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          size="sm"
          disabled={isApplying}
          className="cursor-pointer"
          data-testid="system-updates-apply"
        >
          {isApplying ? (
            <Spinner className="size-3.5 mr-1" />
          ) : (
            <IconDownload className="h-3.5 w-3.5 mr-1" />
          )}
          Apply update
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Apply update?</AlertDialogTitle>
          <AlertDialogDescription className="text-left">
            Kandev will update to {latest}, reinstall the user service, and restart it.
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
