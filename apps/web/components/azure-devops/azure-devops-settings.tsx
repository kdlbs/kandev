"use client";

import { useCallback, useEffect, useState } from "react";
import {
  IconBrandAzure,
  IconDeviceFloppy,
  IconExternalLink,
  IconPlugConnected,
  IconTrash,
} from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { SettingsSection } from "@/components/settings/settings-section";
import { useToast } from "@/components/toast-provider";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { useAzureDevOpsProjects } from "@/hooks/domains/azure-devops/use-azure-devops-projects";
import {
  deleteAzureDevOpsConfig,
  getAzureDevOpsConfig,
  setAzureDevOpsConfig,
  testAzureDevOpsConnection,
} from "@/lib/api/domains/azure-devops-api";
import type {
  AzureDevOpsConfig,
  SetAzureDevOpsConfigRequest,
  TestAzureDevOpsConnectionResult,
} from "@/lib/types/azure-devops";

type FormState = {
  organizationUrl: string;
  defaultProjectId: string;
  defaultProjectName: string;
  pat: string;
};

const EMPTY_FORM: FormState = {
  organizationUrl: "",
  defaultProjectId: "",
  defaultProjectName: "",
  pat: "",
};

function configToForm(config: AzureDevOpsConfig | null): FormState {
  if (!config) return EMPTY_FORM;
  return {
    organizationUrl: config.organizationUrl,
    defaultProjectId: config.defaultProjectId ?? "",
    defaultProjectName: config.defaultProjectName ?? "",
    pat: "",
  };
}

function configToHealth(config: AzureDevOpsConfig | null): IntegrationAuthHealth | null {
  if (!config?.hasSecret) return null;
  return {
    ok: config.lastOk,
    error: config.lastError ?? "",
    checkedAt: config.lastCheckedAt ? new Date(config.lastCheckedAt) : null,
  };
}

function normalizedOrganization(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

function azureDevOpsPATURL(value: string): string | null {
  let organizationURL: URL;
  try {
    organizationURL = new URL(value.trim());
  } catch {
    return null;
  }

  const organization = organizationURL.pathname.split("/").filter(Boolean);
  if (
    organizationURL.protocol !== "https:" ||
    organizationURL.hostname.toLowerCase() !== "dev.azure.com" ||
    organizationURL.port ||
    organizationURL.username ||
    organizationURL.password ||
    organizationURL.search ||
    organizationURL.hash ||
    organization.length !== 1 ||
    !/^[a-z0-9-]+$/i.test(organization[0])
  ) {
    return null;
  }

  return `https://dev.azure.com/${organization[0]}/_usersSettings/tokens`;
}

function savedPATMatches(config: AzureDevOpsConfig | null, form: FormState): boolean {
  return (
    !!config?.hasSecret &&
    normalizedOrganization(config.organizationUrl) === normalizedOrganization(form.organizationUrl)
  );
}

function requestFromForm(
  form: FormState,
  savedConfig: AzureDevOpsConfig | null,
): SetAzureDevOpsConfigRequest {
  const organizationMatches =
    normalizedOrganization(form.organizationUrl) ===
    normalizedOrganization(savedConfig?.organizationUrl ?? "");
  return {
    organizationUrl: form.organizationUrl.trim(),
    defaultProjectId: organizationMatches ? form.defaultProjectId : undefined,
    defaultProjectName: organizationMatches ? form.defaultProjectName : undefined,
    authMethod: "pat",
    pat: form.pat || undefined,
  };
}

function useConfigRefresh(
  workspaceId: string,
  setConfig: (config: AzureDevOpsConfig | null) => void,
) {
  useEffect(() => {
    const interval = setInterval(() => {
      getAzureDevOpsConfig(workspaceId)
        .then(setConfig)
        .catch(() => undefined);
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(interval);
  }, [setConfig, workspaceId]);
}

function useAzureDevOpsSettings(workspaceId: string) {
  const { toast } = useToast();
  const [config, setConfig] = useState<AzureDevOpsConfig | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestAzureDevOpsConnectionResult | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const next = await getAzureDevOpsConfig(workspaceId, { cache: "no-store" });
      setConfig(next);
      setForm(configToForm(next));
    } catch (err) {
      toast({
        description: `Failed to load Azure DevOps config: ${String(err)}`,
        variant: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [toast, workspaceId]);

  useEffect(() => void load(), [load]);
  useConfigRefresh(workspaceId, setConfig);

  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((current) => ({
        ...current,
        [key]: value,
        ...(key === "organizationUrl" &&
        normalizedOrganization(String(value)) !== normalizedOrganization(current.organizationUrl)
          ? { defaultProjectId: "", defaultProjectName: "" }
          : {}),
      })),
    [],
  );

  const test = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      setTestResult(await testAzureDevOpsConnection(workspaceId, requestFromForm(form, config)));
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [config, form, workspaceId]);

  const save = useCallback(async () => {
    setSaving(true);
    try {
      const next = await setAzureDevOpsConfig(workspaceId, requestFromForm(form, config));
      setConfig(next);
      setForm(configToForm(next));
      setTestResult(null);
      toast({ description: "Azure DevOps configuration saved", variant: "success" });
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
    } finally {
      setSaving(false);
    }
  }, [config, form, toast, workspaceId]);

  const remove = useCallback(async () => {
    if (!confirm("Remove Azure DevOps configuration?")) return;
    try {
      await deleteAzureDevOpsConfig(workspaceId);
      setConfig(null);
      setForm(EMPTY_FORM);
      setTestResult(null);
      toast({ description: "Azure DevOps configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Remove failed: ${String(err)}`, variant: "error" });
    }
  }, [toast, workspaceId]);

  return {
    config,
    form,
    loading,
    saving,
    testing,
    testResult,
    health: configToHealth(config),
    update,
    test,
    save,
    remove,
  };
}

function TestResult({ result }: { result: TestAzureDevOpsConnectionResult | null }) {
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"} data-testid="azure-devops-test-result">
      <AlertDescription>
        {result.ok
          ? `Connected${result.displayName ? ` as ${result.displayName}` : ""}`
          : result.error || "Connection failed"}
      </AlertDescription>
    </Alert>
  );
}

type SettingsState = ReturnType<typeof useAzureDevOpsSettings>;
type ProjectsState = ReturnType<typeof useAzureDevOpsProjects>;

function PATSetupHelp({ organizationUrl }: { organizationUrl: string }) {
  const patURL = azureDevOpsPATURL(organizationUrl);

  return (
    <div
      id="azure-devops-pat-help"
      className="space-y-2 text-sm"
      data-testid="azure-devops-pat-help"
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="font-medium">Create a read-only personal access token</p>
          <p className="text-muted-foreground">
            Use a short expiration and grant only the access Kandev needs.
          </p>
        </div>
        {patURL ? (
          <Button
            asChild
            variant="outline"
            className="min-h-11 w-full cursor-pointer sm:min-h-0 sm:w-auto"
          >
            <a href={patURL} target="_blank" rel="noreferrer">
              <IconExternalLink className="h-4 w-4" />
              Create personal access token
            </a>
          </Button>
        ) : (
          <p className="text-xs text-muted-foreground">
            Enter a valid organization URL to open its token settings.
          </p>
        )}
      </div>
      <ol className="list-decimal space-y-1 pl-5 text-muted-foreground">
        <li>Open token settings and select New Token.</li>
        <li>Choose this organization, an expiration, and Custom defined scopes.</li>
        <li>
          Under <span className="font-medium text-foreground">Work Items</span>, check Read. Under{" "}
          <span className="font-medium text-foreground">Code</span>, check Read. Leave all other
          scopes unchecked.
        </li>
        <li>Create the token, copy it, and paste it into the field above.</li>
      </ol>
    </div>
  );
}

function ConnectionFields({
  state,
  projects,
  canReusePAT,
  projectSelectionEnabled,
}: {
  state: SettingsState;
  projects: ProjectsState;
  canReusePAT: boolean;
  projectSelectionEnabled: boolean;
}) {
  const projectPlaceholder = projects.loading ? "Loading projects..." : "Optional";
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5 sm:col-span-2">
          <Label htmlFor="azure-devops-organization">Organization URL</Label>
          <Input
            id="azure-devops-organization"
            value={state.form.organizationUrl}
            onChange={(event) => state.update("organizationUrl", event.target.value)}
            placeholder="https://dev.azure.com/organization"
            disabled={state.loading}
            autoComplete="url"
            data-testid="azure-devops-organization"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="azure-devops-project">Default project</Label>
          <Select
            value={state.form.defaultProjectId || undefined}
            onValueChange={(projectId) => {
              const project = projects.data.find((item) => item.id === projectId);
              state.update("defaultProjectId", projectId);
              state.update("defaultProjectName", project?.name ?? "");
            }}
            disabled={
              state.loading ||
              projects.loading ||
              projects.data.length === 0 ||
              !projectSelectionEnabled
            }
          >
            <SelectTrigger id="azure-devops-project" className="w-full">
              <SelectValue placeholder={projectPlaceholder} />
            </SelectTrigger>
            <SelectContent>
              {projects.data.map((project) => (
                <SelectItem key={project.id} value={project.id}>
                  {project.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="azure-devops-pat">Personal Access Token</Label>
          <Input
            id="azure-devops-pat"
            type="password"
            value={state.form.pat}
            onChange={(event) => state.update("pat", event.target.value)}
            placeholder={canReusePAT ? "Saved credential" : "Paste PAT"}
            disabled={state.loading}
            autoComplete="new-password"
            aria-describedby="azure-devops-pat-help"
            data-testid="azure-devops-pat"
          />
        </div>
      </div>
      <PATSetupHelp organizationUrl={state.form.organizationUrl} />
      {projects.error && (
        <p className="text-sm text-destructive" role="alert">
          {projects.error}
        </p>
      )}
    </>
  );
}

function saveButtonLabel(state: SettingsState): string {
  if (state.saving) return "Saving...";
  return state.config ? "Update" : "Save";
}

function ConnectionActions({ state, disabled }: { state: SettingsState; disabled: boolean }) {
  return (
    <div className="flex flex-col-reverse gap-2 sm:flex-row sm:flex-wrap sm:items-center">
      <Button
        type="button"
        variant="outline"
        onClick={() => void state.test()}
        disabled={disabled || state.testing}
        className="w-full cursor-pointer sm:w-auto"
        data-testid="azure-devops-test-button"
      >
        <IconPlugConnected className="h-4 w-4" />
        {state.testing ? "Testing..." : "Test connection"}
      </Button>
      <Button
        type="button"
        onClick={() => void state.save()}
        disabled={disabled || state.saving}
        className="w-full cursor-pointer sm:w-auto"
        data-testid="azure-devops-save-button"
      >
        <IconDeviceFloppy className="h-4 w-4" />
        {saveButtonLabel(state)}
      </Button>
      {state.config && (
        <Button
          type="button"
          variant="destructive"
          onClick={() => void state.remove()}
          className="w-full cursor-pointer sm:ml-auto sm:w-auto"
          data-testid="azure-devops-delete-button"
        >
          <IconTrash className="h-4 w-4" />
          Remove
        </Button>
      )}
    </div>
  );
}

export function AzureDevOpsConnectionSection({ workspaceId }: { workspaceId: string }) {
  const state = useAzureDevOpsSettings(workspaceId);
  const projects = useAzureDevOpsProjects(workspaceId, !!state.config?.hasSecret);
  const canReusePAT = savedPATMatches(state.config, state.form);
  const projectSelectionEnabled =
    !!state.config?.hasSecret &&
    normalizedOrganization(state.config.organizationUrl) ===
      normalizedOrganization(state.form.organizationUrl);
  const missingPAT = !canReusePAT && !state.form.pat;
  const disabled = state.loading || !state.form.organizationUrl || missingPAT;

  return (
    <SettingsSection
      icon={<IconBrandAzure className="h-5 w-5" />}
      title="Azure DevOps integration"
      description="Azure DevOps Services organization, project, and read-only PAT for this workspace."
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <IntegrationAuthStatusBanner health={state.health} />
          <ConnectionFields
            state={state}
            projects={projects}
            canReusePAT={canReusePAT}
            projectSelectionEnabled={projectSelectionEnabled}
          />
          <TestResult result={state.testResult} />
          <Separator />
          <ConnectionActions state={state} disabled={disabled} />
        </CardContent>
      </Card>
    </SettingsSection>
  );
}

export function AzureDevOpsIntegrationPage({ workspaceId }: { workspaceId?: string } = {}) {
  return (
    <WorkspaceScopedSection workspaceId={workspaceId}>
      {(selectedWorkspaceId) => (
        <AzureDevOpsConnectionSection key={selectedWorkspaceId} workspaceId={selectedWorkspaceId} />
      )}
    </WorkspaceScopedSection>
  );
}
