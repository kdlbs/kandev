"use client";

import { useState } from "react";
import {
  IconAlertTriangle,
  IconArrowRight,
  IconBrandGithub,
  IconCheck,
  IconExternalLink,
  IconInfoCircle,
  IconRefresh,
  IconTrash,
} from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";

import Link from "@/components/routing/app-link";
import { useToast } from "@/components/toast-provider";
import { useDeploymentAppRegistration } from "@/hooks/domains/github/use-deployment-app-registration";
import type {
  DeploymentGitHubAppRegistration,
  DeploymentGitHubAppStatus,
  StartDeploymentGitHubAppRequest,
  StartDeploymentGitHubAppResponse,
} from "@/lib/types/github";
import { useSearchParams } from "@/lib/routing/client-router";
import { callbackNoticeForResult, manifestFormFields } from "./github-app-settings-model";
import { GitHubAppSetupForm } from "./github-app-setup-form";

export function GitHubAppSettings() {
  const { status, loading, mutating, error, reload, start, remove } =
    useDeploymentAppRegistration();
  const [handoff, setHandoff] = useState<StartDeploymentGitHubAppResponse | null>(null);
  const { toast } = useToast();
  const callbackNotice = callbackNoticeForResult(useSearchParams().get("github_app_result"));

  const beginSetup = async (request: StartDeploymentGitHubAppRequest) => {
    try {
      setHandoff(await start(request));
    } catch (startError) {
      toast({
        description: startError instanceof Error ? startError.message : "GitHub App setup failed",
        variant: "error",
      });
    }
  };

  const removeRegistration = async () => {
    try {
      const result = await remove();
      if (result.refreshed) {
        toast({ description: "Deployment GitHub App removed", variant: "success" });
      } else {
        toast({ description: "App removed, but its status could not be refreshed. Try again." });
      }
    } catch (removeError) {
      toast({
        description:
          removeError instanceof Error ? removeError.message : "GitHub App could not be removed",
        variant: "error",
      });
    }
  };

  return (
    <div className="min-w-0 space-y-6 overflow-x-hidden" data-testid="github-app-settings">
      {callbackNotice && <CallbackNotice notice={callbackNotice} />}
      <IdentityExplanation />
      <Separator />
      {loading && <p className="text-sm text-muted-foreground">Loading GitHub App status...</p>}
      {error && <StatusLoadError message={error} onRetry={() => void reload()} />}
      {status && !loading && !error && (
        <DeploymentStatus
          status={status}
          busy={mutating}
          onStart={beginSetup}
          onRemove={removeRegistration}
        />
      )}
      <ManifestHandoffDialog handoff={handoff} onOpenChange={(open) => !open && setHandoff(null)} />
    </div>
  );
}

function IdentityExplanation() {
  return (
    <section className="space-y-3" aria-labelledby="github-app-identity-heading">
      <div className="space-y-1">
        <h3 id="github-app-identity-heading" className="text-base font-semibold">
          One App for this Kandev deployment
        </h3>
        <p className="max-w-3xl text-sm text-muted-foreground">
          This App is the organization-managed automation identity available to workspaces. It is
          separate from the personal identity used for My GitHub and human-attributed actions.
        </p>
      </div>
      <div className="divide-y rounded-md border text-sm">
        <IdentityRow
          title="Deployment GitHub App"
          detail="Short-lived, repository-scoped credentials for background jobs and managed agents."
        />
        <IdentityRow
          title="Workspace PAT or GitHub CLI"
          detail="A human account selected to perform automation for one workspace."
        />
        <IdentityRow
          title="My GitHub identity"
          detail="An optional personal connection used for viewer-specific and human-attributed actions; agents never receive it."
        />
      </div>
    </section>
  );
}

function IdentityRow({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="grid gap-1 p-3 sm:grid-cols-[12rem_minmax(0,1fr)] sm:gap-4">
      <p className="font-medium">{title}</p>
      <p className="min-w-0 text-muted-foreground">{detail}</p>
    </div>
  );
}

function DeploymentStatus({
  status,
  busy,
  onStart,
  onRemove,
}: {
  status: DeploymentGitHubAppStatus;
  busy: boolean;
  onStart: (request: StartDeploymentGitHubAppRequest) => Promise<void>;
  onRemove: () => Promise<void>;
}) {
  return (
    <section
      className="space-y-5"
      aria-labelledby="github-app-status-heading"
      data-testid="github-app-status"
      data-source={status.source}
      data-state={status.state}
      data-webhook-status={status.registration?.webhook_status ?? "none"}
    >
      <StatusHeading status={status} />
      {status.state === "invalid" && <InvalidStatus status={status} />}
      {status.ready && status.registration && (
        <ReadyRegistration registration={status.registration} readOnly={status.read_only} />
      )}
      {!status.ready && status.source === "none" && (
        <SetupSection registering={status.state === "registering"} busy={busy} onStart={onStart} />
      )}
      {status.source === "managed" && <RemoveRegistration busy={busy} onRemove={onRemove} />}
    </section>
  );
}

function StatusHeading({ status }: { status: DeploymentGitHubAppStatus }) {
  const label = deploymentStatusLabel(status);
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="space-y-1">
        <h3 id="github-app-status-heading" className="text-base font-semibold">
          Deployment App status
        </h3>
        <p className="text-sm text-muted-foreground">
          {status.unavailable_reason ??
            "Registration metadata and webhook health for this deployment."}
        </p>
      </div>
      <Badge variant={status.state === "invalid" ? "destructive" : "outline"}>{label}</Badge>
    </div>
  );
}

function deploymentStatusLabel(status: DeploymentGitHubAppStatus): string {
  if (status.ready && status.source === "environment") return "Externally managed";
  if (status.ready) return "Managed by Kandev";
  if (status.state === "registering") return "Setup in progress";
  if (status.state === "invalid") return "Unavailable";
  return "Not configured";
}

function SetupSection({
  registering,
  busy,
  onStart,
}: {
  registering: boolean;
  busy: boolean;
  onStart: (request: StartDeploymentGitHubAppRequest) => Promise<void>;
}) {
  return (
    <div className="space-y-5">
      {registering && (
        <Alert>
          <IconRefresh className="h-4 w-4" />
          <AlertTitle>Waiting for GitHub</AlertTitle>
          <AlertDescription>
            Finish the App form on GitHub. You can start again here if that browser flow was closed.
          </AlertDescription>
        </Alert>
      )}
      <div className="space-y-1">
        <h4 className="text-sm font-semibold">
          {registering ? "Start setup again" : "Create the App"}
        </h4>
        <p className="text-xs text-muted-foreground">
          Kandev generates the required policy. GitHub asks you to confirm it before creating the
          App.
        </p>
      </div>
      <GitHubAppSetupForm busy={busy} onStart={onStart} />
    </div>
  );
}

function InvalidStatus({ status }: { status: DeploymentGitHubAppStatus }) {
  return (
    <Alert variant="destructive" data-testid="github-app-invalid-status">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertTitle>GitHub App configuration is invalid</AlertTitle>
      <AlertDescription>
        {status.unavailable_reason}
        {status.read_only
          ? " Update the KANDEV_GITHUB_APP_* environment variables and restart Kandev."
          : " Remove the managed registration below, then create it again."}
      </AlertDescription>
    </Alert>
  );
}

function ReadyRegistration({
  registration,
  readOnly,
}: {
  registration: DeploymentGitHubAppRegistration;
  readOnly: boolean;
}) {
  return (
    <div className="space-y-5">
      {readOnly && (
        <Alert data-testid="github-app-environment-status">
          <IconBrandGithub className="h-4 w-4" />
          <AlertTitle>Managed outside Kandev</AlertTitle>
          <AlertDescription>
            Environment configuration has priority. This page cannot replace or remove it.
          </AlertDescription>
        </Alert>
      )}
      <dl className="divide-y rounded-md border text-sm">
        <Metadata label="App" value={registration.slug || `App ${registration.app_id}`} />
        <Metadata
          label="Owner"
          value={registration.owner_login || (readOnly ? "Configured externally" : "Unknown")}
        />
        <Metadata label="GitHub host" value={registration.github_host || "github.com"} />
        <Metadata label="Public URL" value={registration.public_base_url} />
      </dl>
      <WebhookHealth registration={registration} />
      <div className="space-y-2">
        <h4 className="text-sm font-semibold">Use the App in a workspace</h4>
        <p className="text-xs text-muted-foreground">
          The deployment App is available now. Each workspace still chooses and installs its own
          automation connection.
        </p>
        <Button
          asChild
          className="min-h-11 w-full sm:w-auto"
          data-testid="github-app-workspace-handoff"
        >
          <Link href="/settings/integrations/github">
            Open workspace GitHub settings
            <IconArrowRight className="ml-2 h-4 w-4" />
          </Link>
        </Button>
      </div>
    </div>
  );
}

function Metadata({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="grid min-w-0 gap-1 p-3 sm:grid-cols-[10rem_minmax(0,1fr)] sm:gap-4">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="break-words font-medium">{value || "Not reported"}</dd>
    </div>
  );
}

function WebhookHealth({ registration }: { registration: DeploymentGitHubAppRegistration }) {
  const verified = registration.webhook_status === "verified";
  const failing = registration.webhook_status === "failing";
  const copy = webhookHealthCopy(registration);
  return (
    <Alert variant={failing ? "destructive" : "default"} data-testid="github-app-webhook-status">
      {verified ? <IconCheck className="h-4 w-4" /> : <IconAlertTriangle className="h-4 w-4" />}
      <AlertTitle>{copy.title}</AlertTitle>
      <AlertDescription>{copy.description}</AlertDescription>
    </Alert>
  );
}

function webhookHealthCopy(registration: DeploymentGitHubAppRegistration) {
  if (registration.webhook_status === "verified") {
    return {
      title: "Webhook verified",
      description: `A signed GitHub delivery was observed${formatObservedAt(registration.last_webhook_at)}.`,
    };
  }
  if (registration.webhook_status === "failing") {
    return {
      title: "Webhook delivery is failing",
      description:
        registration.last_error || "Kandev could not validate the latest GitHub delivery.",
    };
  }
  return {
    title: "Waiting for webhook",
    description:
      "Kandev marks this verified after it receives the first correctly signed GitHub delivery.",
  };
}

function formatObservedAt(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : ` on ${date.toLocaleString()}`;
}

function RemoveRegistration({ busy, onRemove }: { busy: boolean; onRemove: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) setConfirmation("");
  };
  return (
    <div className="border-t pt-5">
      <Button
        type="button"
        variant="destructive"
        disabled={busy}
        onClick={() => setOpen(true)}
        className="min-h-11 w-full cursor-pointer sm:w-auto"
        data-testid="github-app-remove-button"
      >
        <IconTrash className="mr-2 h-4 w-4" />
        Remove managed App
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
          <AlertDialogHeader>
            <AlertDialogTitle>Remove the deployment GitHub App?</AlertDialogTitle>
            <AlertDialogDescription className="text-left">
              Kandev deletes its encrypted App credentials but does not delete the App on GitHub.
              Workspaces using this App block removal. Type <strong>DELETE</strong> to continue.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Input
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            className="h-12"
            aria-label="Type DELETE to confirm"
            data-testid="github-app-remove-confirmation"
          />
          <AlertDialogFooter>
            <AlertDialogCancel className="min-h-12 cursor-pointer">Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={confirmation !== "DELETE"}
              onClick={() => void onRemove()}
              className="min-h-12 cursor-pointer"
              data-testid="github-app-remove-confirm"
            >
              Remove App credentials
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ManifestHandoffDialog({
  handoff,
  onOpenChange,
}: {
  handoff: StartDeploymentGitHubAppResponse | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <AlertDialog open={Boolean(handoff)} onOpenChange={onOpenChange}>
      <AlertDialogContent
        className="max-w-[calc(100vw-2rem)] sm:max-w-md"
        data-testid="github-app-manifest-confirm"
      >
        <AlertDialogHeader>
          <AlertDialogTitle>Continue to GitHub?</AlertDialogTitle>
          <AlertDialogDescription className="text-left">
            GitHub will show the generated App name, permissions, callbacks, and owner for your
            review. No Kandev secret is sent. This setup request expires in one hour.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="min-h-12 cursor-pointer">Stay in Kandev</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => handoff && submitManifestToGitHub(handoff)}
            className="min-h-12 cursor-pointer"
            data-testid="github-app-manifest-continue"
          >
            Continue to GitHub
            <IconExternalLink className="ml-2 h-4 w-4" />
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function submitManifestToGitHub(handoff: StartDeploymentGitHubAppResponse) {
  const form = document.createElement("form");
  form.method = "POST";
  form.action = handoff.registration_url;
  const fields = manifestFormFields(handoff.manifest);
  for (const [name, value] of Object.entries(fields)) {
    const input = document.createElement("input");
    input.type = "hidden";
    input.name = name;
    input.value = value;
    form.appendChild(input);
  }
  document.body.appendChild(form);
  form.submit();
}

function CallbackNotice({
  notice,
}: {
  notice: NonNullable<ReturnType<typeof callbackNoticeForResult>>;
}) {
  return (
    <Alert
      variant={notice.tone === "error" ? "destructive" : "default"}
      data-testid="github-app-callback-result"
    >
      <CallbackNoticeIcon tone={notice.tone} />
      <AlertTitle>{notice.title}</AlertTitle>
      <AlertDescription>{notice.description}</AlertDescription>
    </Alert>
  );
}

function CallbackNoticeIcon({ tone }: { tone: "success" | "info" | "error" }) {
  if (tone === "error") return <IconAlertTriangle className="h-4 w-4" />;
  if (tone === "info") return <IconInfoCircle className="h-4 w-4" />;
  return <IconCheck className="h-4 w-4" />;
}

function StatusLoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <Alert variant="destructive" data-testid="github-app-status-error">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertTitle>GitHub App status could not be loaded</AlertTitle>
      <AlertDescription>
        {message}
        <Button
          variant="outline"
          onClick={onRetry}
          className="mt-3 min-h-11 w-full cursor-pointer sm:w-auto"
        >
          <IconRefresh className="mr-2 h-4 w-4" />
          Try again
        </Button>
      </AlertDescription>
    </Alert>
  );
}
