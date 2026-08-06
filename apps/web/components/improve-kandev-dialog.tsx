"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
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
import {
  initialImproveKandevMode,
  getImproveKandevBrowserStorage,
  readImproveKandevSkipIntro,
  writeImproveKandevSkipIntro,
} from "./improve-kandev-dialog-model";
import { Trans, useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  const { open, onOpenChange, workspaceId, onSuccess } = props;
  const [mode, setMode] = useState<Mode>(() => initialImproveKandevMode(readSkipIntro()));
  const [skipIntro, setSkipIntro] = useState(() => readSkipIntro());
  const [auth, setAuth] = useState<AuthState>({ kind: "checking" });
  const [bootstrap, setBootstrap] = useState<BootstrapState>({ kind: "idle" });
  const [captureLogs, setCaptureLogs] = useState(true);

  // Reset everything on close so a re-open re-runs the auth check.
  // The setState-in-effect calls here mirror the documented "subscribe to
  // external system" pattern (parent-controlled `open` toggling).
  useEffect(() => {
    if (open) return;
    /* eslint-disable react-hooks/set-state-in-effect */
    setMode(initialImproveKandevMode(readSkipIntro()));
    setSkipIntro(readSkipIntro());
    setAuth({ kind: "checking" });
    setBootstrap({ kind: "idle" });
    setCaptureLogs(true);
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [open]);

  useGitHubAuthCheck(open, workspaceId, setAuth);
  useBootstrapKandev(open, mode, workspaceId, setBootstrap);
  useEffect(() => {
    if (!open || mode !== "create" || auth.kind !== "missing") return;
    // A saved intro preference must not bypass the GitHub-auth recovery UI.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMode("intro");
  }, [auth.kind, mode, open]);

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
        externalBlockedReason={
          auth.kind === "checking" ? t("common:checkingGithubAuthentication") : null
        }
      />
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconStethoscope className="h-5 w-5" />
            {t("common:improveKandev")}
          </DialogTitle>
        </DialogHeader>
        <IntroBody
          auth={auth}
          skipIntro={skipIntro}
          onSkipIntroChange={(checked) => {
            setSkipIntro(checked);
            writeImproveKandevSkipIntro(getImproveKandevBrowserStorage(), checked);
          }}
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
  const { t } = useTranslation();
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
          fixLabel: ghIssue.fix_label || t("common:configureGithub"),
        });
      } catch {
        if (!cancelled) setAuth({ kind: "ok" }); // Fail open — bootstrap will surface real errors.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, setAuth, t]);
}

function useBootstrapKandev(
  open: boolean,
  mode: Mode,
  workspaceId: string | null,
  setBootstrap: (s: BootstrapState) => void,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const setRepositories = useAppStore((state) => state.setRepositories);
  useEffect(() => {
    if (!open || mode !== "create" || !workspaceId) return;
    let cancelled = false;
    setBootstrap({ kind: "loading" });
    (async () => {
      try {
        const data = await bootstrapImproveKandev(workspaceId);
        // Refresh the workspace repository list so the newly-created kandev
        // repo is in the store; otherwise the locked repo dropdown can't
        // resolve a label for the bootstrapped repository_id.
        const [stepsRes, issueStepsRes, reposRes] = await Promise.all([
          listWorkflowSteps(data.workflow_id),
          listWorkflowSteps(data.issue_workflow_id),
          listRepositories(workspaceId, undefined, { cache: "no-store" }),
        ]);
        if (cancelled) return;
        setRepositories(workspaceId, reposRes.repositories);
        setBootstrap({
          kind: "ready",
          data,
          steps: stepsRes.steps,
          issueSteps: issueStepsRes.steps,
        });
      } catch (err) {
        if (cancelled) return;
        const message = err instanceof Error ? err.message : t("common:bootstrapFailed");
        setBootstrap({ kind: "error", message });
        toast({
          title: t("common:couldNotPrepareImproveKandev"),
          description: message,
          variant: "error",
        });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, mode, workspaceId, setBootstrap, setRepositories, toast, t]);
}

function IntroBody({
  auth,
  skipIntro,
  onSkipIntroChange,
  onCancel,
  onProceed,
}: {
  auth: AuthState;
  skipIntro: boolean;
  onSkipIntroChange: (checked: boolean) => void;
  onCancel: () => void;
  onProceed: () => void;
}) {
  if (auth.kind === "missing") {
    return <GhAuthMissing auth={auth} onCancel={onCancel} />;
  }
  return (
    <IntroExplanation
      auth={auth}
      skipIntro={skipIntro}
      onSkipIntroChange={onSkipIntroChange}
      onCancel={onCancel}
      onProceed={onProceed}
    />
  );
}

function GhAuthMissing({
  auth,
  onCancel,
}: {
  auth: Extract<AuthState, { kind: "missing" }>;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4 py-2">
      <div className="flex items-start gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
        <IconAlertTriangle className="h-4 w-4 shrink-0 text-amber-500" />
        <div>
          <p className="font-medium text-foreground">{t("common:githubCliNotAuthenticated")}</p>
          <p className="mt-1 text-muted-foreground">
            {/* `gh` is the CLI's binary name, so it stays literal in the children
                and never becomes a catalog key. */}
            <Trans
              i18nKey="common:theFinalStepOpensAPullRequest"
              values={{ message: auth.message }}
            >
              The final step of this workflow opens a pull request, which needs the <code>gh</code>{" "}
              CLI to be authenticated. {auth.message}
            </Trans>
          </p>
        </div>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} className="cursor-pointer">
          {t("common:cancel")}
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
  skipIntro,
  onSkipIntroChange,
  onCancel,
  onProceed,
}: {
  auth: AuthState;
  skipIntro: boolean;
  onSkipIntroChange: (checked: boolean) => void;
  onCancel: () => void;
  onProceed: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-5 py-2">
      <p className="text-sm leading-relaxed text-muted-foreground">
        {t("common:kandevIsOpenSourceAndYou")}
      </p>

      <p className="text-sm leading-relaxed text-muted-foreground">
        {t("common:describeABugYouHitOr")}
      </p>

      <p className="text-sm leading-relaxed text-muted-foreground">
        <Trans i18nKey="common:whenItsDoneTheAgentOpensAPr" values={{ repo: "kdlbs/kandev" }}>
          When it&apos;s done, the agent opens a pull request to{" "}
          <code className="font-mono text-xs">kdlbs/kandev</code> for the maintainers to review,
          saving them time and shipping the improvement to everyone.
        </Trans>
      </p>

      <ul className="space-y-2 text-sm text-muted-foreground">
        <IntroBullet>{t("common:createATaskDescribingYourBug")}</IntroBullet>
        <IntroBullet>{t("common:yourAgentImplementsItInThe")}</IntroBullet>
        <IntroBullet>{t("common:youVerifyAndTestTheChange")}</IntroBullet>
        <IntroBullet>
          <Trans
            i18nKey="common:theAgentForksKandevToYourAccount"
            values={{ repo: "kdlbs/kandev" }}
          >
            The agent forks <code className="font-mono text-xs">kdlbs/kandev</code> to your GitHub
            account and opens a PR from your fork, credited to you
          </Trans>
        </IntroBullet>
      </ul>
      <label
        className="flex min-h-12 cursor-pointer items-center gap-2 text-sm text-muted-foreground"
        data-testid="improve-kandev-skip-intro"
      >
        <Checkbox
          checked={skipIntro}
          onCheckedChange={(checked) => onSkipIntroChange(checked === true)}
        />
        {t("common:doNotShowThisAgain")}
      </label>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} className="cursor-pointer">
          {t("common:cancel")}
        </Button>
        <Button
          onClick={onProceed}
          disabled={auth.kind === "checking"}
          className="cursor-pointer"
          data-testid="improve-kandev-proceed"
        >
          {t("common:contribute")}
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

function readSkipIntro(): boolean {
  try {
    return readImproveKandevSkipIntro(globalThis.localStorage);
  } catch {
    return false;
  }
}
