"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { IconAlertTriangle, IconStethoscope, IconCheck } from "@tabler/icons-react";

import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { bootstrapImproveKandev } from "@/lib/api/domains/improve-kandev-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { fetchSystemHealth } from "@/lib/api/domains/health-api";
import type { Task } from "@/lib/types/http";
import { buildImproveKandevDescription } from "./improve-kandev-dialog-helpers";
import { CreateModeView, type BootstrapState } from "./improve-kandev-dialog-create";

type ImproveKandevDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  onSuccess?: (task: Task) => void;
};

type Mode = "intro" | "create";

type AuthState =
  | { kind: "checking" }
  | { kind: "ok" }
  | { kind: "missing"; message: string; fixUrl: string; fixLabel: string };

export function ImproveKandevDialog(props: ImproveKandevDialogProps) {
  const { open, onOpenChange, workspaceId, onSuccess } = props;
  const [mode, setMode] = useState<Mode>("intro");
  const [auth, setAuth] = useState<AuthState>({ kind: "checking" });
  const [bootstrap, setBootstrap] = useState<BootstrapState>({ kind: "idle" });
  const [captureLogs, setCaptureLogs] = useState(true);

  // Reset everything on close so a re-open re-runs the auth check.
  // The setState-in-effect calls here mirror the documented "subscribe to
  // external system" pattern (parent-controlled `open` toggling).
  useEffect(() => {
    if (open) return;
    /* eslint-disable react-hooks/set-state-in-effect */
    setMode("intro");
    setAuth({ kind: "checking" });
    setBootstrap({ kind: "idle" });
    setCaptureLogs(true);
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [open]);

  useGitHubAuthCheck(open, workspaceId, setAuth);
  useBootstrapKandev(mode, workspaceId, setBootstrap);

  const handleSuccess = useCallback(
    (task: Task) => {
      onOpenChange(false);
      onSuccess?.(task);
    },
    [onOpenChange, onSuccess],
  );

  const transformDescription = useCallback(
    async (description: string) => {
      if (bootstrap.kind !== "ready") return description;
      return buildImproveKandevDescription(description, bootstrap.data, captureLogs);
    },
    [bootstrap, captureLogs],
  );

  if (mode === "create") {
    return (
      <CreateModeView
        open={open}
        onOpenChange={onOpenChange}
        workspaceId={workspaceId}
        bootstrap={bootstrap}
        captureLogs={captureLogs}
        setCaptureLogs={setCaptureLogs}
        transformDescription={transformDescription}
        onTaskCreated={handleSuccess}
      />
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconStethoscope className="h-5 w-5" />
            Improve Kandev
          </DialogTitle>
        </DialogHeader>
        <IntroBody
          auth={auth}
          onCancel={() => onOpenChange(false)}
          onProceed={() => setMode("create")}
        />
      </DialogContent>
    </Dialog>
  );
}

function useGitHubAuthCheck(
  open: boolean,
  workspaceId: string | null,
  setAuth: (s: AuthState) => void,
) {
  useEffect(() => {
    if (!open || !workspaceId) return;
    let cancelled = false;
    (async () => {
      try {
        const health = await fetchSystemHealth();
        if (cancelled) return;
        const ghIssue = health.issues.find((i) => i.category === "github");
        if (!ghIssue) {
          setAuth({ kind: "ok" });
          return;
        }
        setAuth({
          kind: "missing",
          message: ghIssue.message,
          fixUrl: ghIssue.fix_url.replace("{workspaceId}", workspaceId),
          fixLabel: ghIssue.fix_label || "Configure GitHub",
        });
      } catch {
        if (!cancelled) setAuth({ kind: "ok" }); // Fail open — bootstrap will surface real errors.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, setAuth]);
}

function useBootstrapKandev(
  mode: Mode,
  workspaceId: string | null,
  setBootstrap: (s: BootstrapState) => void,
) {
  const { toast } = useToast();
  const setRepositories = useAppStore((state) => state.setRepositories);
  useEffect(() => {
    if (mode !== "create" || !workspaceId) return;
    let cancelled = false;
    setBootstrap({ kind: "loading" });
    (async () => {
      try {
        const data = await bootstrapImproveKandev(workspaceId);
        // Surface the EMU fork-restriction case before any further setup so
        // the user sees a clear error and can't submit a contribution that
        // would only fail at the PR step.
        if (data.fork_status === "blocked_emu") {
          if (cancelled) return;
          const message = data.fork_message || "Your account cannot fork kdlbs/kandev.";
          setBootstrap({ kind: "blocked", message });
          toast({
            title: "Cannot contribute from this account",
            description: message,
            variant: "error",
          });
          return;
        }
        // Refresh the workspace repository list so the newly-created kandev
        // repo is in the store; otherwise the locked repo dropdown can't
        // resolve a label for the bootstrapped repository_id.
        const [stepsRes, reposRes] = await Promise.all([
          listWorkflowSteps(data.workflow_id),
          listRepositories(workspaceId, undefined, { cache: "no-store" }),
        ]);
        if (cancelled) return;
        setRepositories(workspaceId, reposRes.repositories);
        setBootstrap({ kind: "ready", data, steps: stepsRes.steps });
      } catch (err) {
        if (cancelled) return;
        const message = err instanceof Error ? err.message : "Bootstrap failed";
        setBootstrap({ kind: "error", message });
        toast({
          title: "Could not prepare Improve Kandev",
          description: message,
          variant: "error",
        });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [mode, workspaceId, setBootstrap, setRepositories, toast]);
}

function IntroBody({
  auth,
  onCancel,
  onProceed,
}: {
  auth: AuthState;
  onCancel: () => void;
  onProceed: () => void;
}) {
  if (auth.kind === "missing") {
    return <GhAuthMissing auth={auth} onCancel={onCancel} />;
  }
  return <IntroExplanation auth={auth} onCancel={onCancel} onProceed={onProceed} />;
}

function GhAuthMissing({
  auth,
  onCancel,
}: {
  auth: Extract<AuthState, { kind: "missing" }>;
  onCancel: () => void;
}) {
  return (
    <div className="space-y-4 py-2">
      <div className="flex items-start gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
        <IconAlertTriangle className="h-4 w-4 shrink-0 text-amber-500" />
        <div>
          <p className="font-medium text-foreground">GitHub CLI not authenticated</p>
          <p className="mt-1 text-muted-foreground">
            The final step of this workflow opens a pull request, which needs the <code>gh</code>{" "}
            CLI to be authenticated. {auth.message}
          </p>
        </div>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} className="cursor-pointer">
          Cancel
        </Button>
        <Button asChild className="cursor-pointer">
          <Link href={auth.fixUrl} onClick={onCancel}>
            {auth.fixLabel}
          </Link>
        </Button>
      </div>
    </div>
  );
}

function IntroExplanation({
  auth,
  onCancel,
  onProceed,
}: {
  auth: AuthState;
  onCancel: () => void;
  onProceed: () => void;
}) {
  return (
    <div className="space-y-5 py-2">
      <p className="text-sm leading-relaxed text-muted-foreground">
        KanDev is open source, and you can help make it better.
      </p>

      <p className="text-sm leading-relaxed text-muted-foreground">
        Describe a bug you hit or a feature you&apos;d like, and we&apos;ll create a task on your
        own agent to implement it in the kandev codebase.
      </p>

      <p className="text-sm leading-relaxed text-muted-foreground">
        When it&apos;s done, the agent opens a pull request to{" "}
        <code className="font-mono text-xs">kdlbs/kandev</code> for the maintainers to review,
        saving them time and shipping the improvement to everyone.
      </p>

      <ul className="space-y-2 text-sm text-muted-foreground">
        <IntroBullet>Create a task describing your bug or feature request</IntroBullet>
        <IntroBullet>Your agent implements it in the kandev repository, with tests</IntroBullet>
        <IntroBullet>You verify and test the change in a second kandev instance</IntroBullet>
        <IntroBullet>
          The agent forks <code className="font-mono text-xs">kdlbs/kandev</code> to your GitHub
          account and opens a PR from your fork, credited to you
        </IntroBullet>
      </ul>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} className="cursor-pointer">
          Cancel
        </Button>
        <Button
          onClick={onProceed}
          disabled={auth.kind === "checking"}
          className="cursor-pointer"
          data-testid="improve-kandev-proceed"
        >
          Contribute
        </Button>
      </div>
    </div>
  );
}

function IntroBullet({ children }: { children: React.ReactNode }) {
  return (
    <li className="flex items-start gap-2">
      <IconCheck className="h-4 w-4 mt-0.5 shrink-0 text-emerald-500" />
      <span>{children}</span>
    </li>
  );
}
