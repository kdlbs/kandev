"use client";

import { useCallback, useEffect, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import {
  IconBrandAzure,
  IconDeviceFloppy,
  IconExternalLink,
  IconInfoCircle,
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
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { AzureDevOpsDefaultQueriesSection } from "@/components/azure-devops/azure-devops-default-queries";
import { AzureDevOpsQuickActionsSection } from "@/components/azure-devops/azure-devops-quick-actions";
import { AzureDevOpsWatchSettings } from "@/components/azure-devops/azure-devops-watch-settings";
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
    organizationUrl: normalizedOrganization(form.organizationUrl),
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
  const { t } = useTranslation();
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
        description: t("azuredevops:failedToLoadConfig", { error: String(err) }),
        variant: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [t, toast, workspaceId]);

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
      toast({ description: t("azuredevops:configurationSaved"), variant: "success" });
    } catch (err) {
      toast({
        description: t("azuredevops:saveFailed", { error: String(err) }),
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  }, [config, form, t, toast, workspaceId]);

  const remove = useCallback(async () => {
    if (!confirm(t("azuredevops:removeConfigurationConfirm"))) return;
    try {
      await deleteAzureDevOpsConfig(workspaceId);
      setConfig(null);
      setForm(EMPTY_FORM);
      setTestResult(null);
      toast({ description: t("azuredevops:configurationRemoved"), variant: "success" });
    } catch (err) {
      toast({
        description: t("azuredevops:removeFailed", { error: String(err) }),
        variant: "error",
      });
    }
  }, [t, toast, workspaceId]);

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

function testResultMessage(
  t: (key: string, values?: Record<string, unknown>) => string,
  result: TestAzureDevOpsConnectionResult,
): string {
  if (!result.ok) return result.error || t("azuredevops:connectionFailed");
  // `displayName` is the Azure DevOps identity — data, so it is interpolated
  // rather than concatenated into the message.
  return result.displayName
    ? t("azuredevops:connectedAs", { name: result.displayName })
    : t("azuredevops:connected");
}

function TestResult({ result }: { result: TestAzureDevOpsConnectionResult | null }) {
  const { t } = useTranslation();
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"} data-testid="azure-devops-test-result">
      <AlertDescription>{testResultMessage(t, result)}</AlertDescription>
    </Alert>
  );
}

type SettingsState = ReturnType<typeof useAzureDevOpsSettings>;
type ProjectsState = ReturnType<typeof useAzureDevOpsProjects>;

// Azure DevOps names these two PAT scope groups in its own token UI. They are
// pointers into that screen, not copy, so they are interpolated as values — the
// pseudo-locale must not transliterate a label the user has to find verbatim.
const PAT_SCOPE_WORK_ITEMS = "Work Items";
const PAT_SCOPE_CODE = "Code";

function PATSetupHelp({ organizationUrl }: { organizationUrl: string }) {
  const { t } = useTranslation();
  const patURL = azureDevOpsPATURL(organizationUrl);
  const [open, setOpen] = useState(false);

  return (
    <TooltipProvider disableHoverableContent={false}>
      <Tooltip open={open} onOpenChange={setOpen}>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="size-11 shrink-0 cursor-help text-muted-foreground sm:size-7"
            aria-label={t("azuredevops:howToCreatePat")}
            onClick={() => setOpen((current) => !current)}
          >
            <IconInfoCircle className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent
          id="azure-devops-pat-help"
          side="top"
          align="end"
          className="pointer-events-auto max-w-sm space-y-2 p-3 text-left text-xs leading-relaxed"
          data-testid="azure-devops-pat-help"
        >
          <p className="font-medium text-foreground">{t("azuredevops:patHelpTitle")}</p>
          <ol className="list-decimal space-y-1 pl-4 text-muted-foreground">
            <li>{t("azuredevops:patStepOpenTokenSettings")}</li>
            <li>{t("azuredevops:patStepChooseScopes")}</li>
            <li>
              <Trans
                i18nKey="azuredevops:patStepCheckScopes"
                values={{ workItems: PAT_SCOPE_WORK_ITEMS, code: PAT_SCOPE_CODE }}
              >
                Under <span className="font-medium text-foreground">{PAT_SCOPE_WORK_ITEMS}</span>,
                check Read &amp; write. Under{" "}
                <span className="font-medium text-foreground">{PAT_SCOPE_CODE}</span>, check Read.
                Leave all other scopes unchecked.
              </Trans>
            </li>
            <li>{t("azuredevops:patStepCreateAndPaste")}</li>
          </ol>
          {patURL ? (
            <a
              href={patURL}
              target="_blank"
              rel="noreferrer"
              className="inline-flex cursor-pointer items-center gap-1 font-medium text-foreground underline underline-offset-4"
            >
              <IconExternalLink className="size-3.5" />
              {t("azuredevops:createPersonalAccessToken")}
            </a>
          ) : (
            <p className="text-muted-foreground">{t("azuredevops:enterValidOrganizationUrl")}</p>
          )}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
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
  const { t } = useTranslation();
  const projectPlaceholder = projects.loading
    ? t("azuredevops:loadingProjects")
    : t("azuredevops:optional");
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5 sm:col-span-2">
          <Label htmlFor="azure-devops-organization">{t("azuredevops:organizationUrl")}</Label>
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
          <div className="sm:flex sm:min-h-7 sm:items-center">
            <Label htmlFor="azure-devops-project">{t("azuredevops:defaultProject")}</Label>
          </div>
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
          <div className="flex items-center gap-1 sm:min-h-7">
            <Label htmlFor="azure-devops-pat">{t("azuredevops:personalAccessToken")}</Label>
            <PATSetupHelp organizationUrl={state.form.organizationUrl} />
          </div>
          <Input
            id="azure-devops-pat"
            type="password"
            value={state.form.pat}
            onChange={(event) => state.update("pat", event.target.value)}
            placeholder={canReusePAT ? t("azuredevops:savedCredential") : t("azuredevops:pastePat")}
            disabled={state.loading}
            autoComplete="new-password"
            aria-describedby="azure-devops-pat-help"
            data-testid="azure-devops-pat"
          />
        </div>
      </div>
      {projects.error && (
        <p className="text-sm text-destructive" role="alert">
          {projects.error}
        </p>
      )}
    </>
  );
}

function saveButtonLabel(t: (key: string) => string, state: SettingsState): string {
  if (state.saving) return t("azuredevops:saving");
  return state.config ? t("azuredevops:update") : t("azuredevops:save");
}

function ConnectionActions({ state, disabled }: { state: SettingsState; disabled: boolean }) {
  const { t } = useTranslation();
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
        {state.testing ? t("azuredevops:testing") : t("azuredevops:testConnection")}
      </Button>
      <Button
        type="button"
        onClick={() => void state.save()}
        disabled={disabled || state.saving}
        className="w-full cursor-pointer sm:w-auto"
        data-testid="azure-devops-save-button"
      >
        <IconDeviceFloppy className="h-4 w-4" />
        {saveButtonLabel(t, state)}
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
          {t("azuredevops:remove")}
        </Button>
      )}
    </div>
  );
}

export function AzureDevOpsConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
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
      title={t("azuredevops:integrationTitle")}
      description={t("azuredevops:integrationDescription")}
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
        <div className="space-y-6">
          <AzureDevOpsConnectionSection
            key={selectedWorkspaceId}
            workspaceId={selectedWorkspaceId}
          />
          <AzureDevOpsWatchSettings workspaceId={selectedWorkspaceId} />
          <AzureDevOpsQuickActionsSection workspaceId={selectedWorkspaceId} />
          <AzureDevOpsDefaultQueriesSection workspaceId={selectedWorkspaceId} />
        </div>
      )}
    </WorkspaceScopedSection>
  );
}
