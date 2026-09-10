"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import { IconArrowLeft, IconDeviceFloppy } from "@tabler/icons-react";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type {
  MCPDefinitionInput,
  MCPExecutionMode,
  MCPSecretBinding,
  MCPServerConfiguration,
  MCPServerDefinition,
  MCPTransport,
} from "@/lib/types/http-mcp";

type FormDraft = {
  runtimeName: string;
  displayName: string;
  description: string;
  mode: MCPExecutionMode;
  transport: MCPTransport;
  command: string;
  args: string;
  url: string;
  packageName: string;
  packageVersion: string;
  configuration: MCPServerConfiguration;
  secretBindings: MCPSecretBinding[];
};

type MCPDefinitionFormProps = {
  workspaceId: string;
  server?: MCPServerDefinition | null;
  onSave: (payload: MCPDefinitionInput, server?: MCPServerDefinition) => Promise<void>;
  onClose: () => void;
};

// i18n-exempt: npm package-shaped example used as a form placeholder.
const MCP_RUNTIME_NAME_PLACEHOLDER = "@vendor/mcp-server";

function emptyDraft(): FormDraft {
  return {
    runtimeName: "",
    displayName: "",
    description: "",
    mode: "existing_executable",
    transport: "stdio",
    command: "",
    args: "",
    url: "",
    packageName: "",
    packageVersion: "",
    configuration: {},
    secretBindings: [],
  };
}

function cloneConfiguration(config: MCPServerConfiguration): MCPServerConfiguration {
  return {
    ...config,
    args: config.args ? [...config.args] : undefined,
    env: config.env ? { ...config.env } : undefined,
    headers: config.headers ? { ...config.headers } : undefined,
    options: config.options ? { ...config.options } : undefined,
    package_runtime_arguments: config.package_runtime_arguments
      ? [...config.package_runtime_arguments]
      : undefined,
    package_arguments: config.package_arguments ? [...config.package_arguments] : undefined,
  };
}

function draftFromServer(server?: MCPServerDefinition | null): FormDraft {
  if (!server) return emptyDraft();
  const config = server.configuration;
  return {
    runtimeName: server.runtime_name,
    displayName: server.display_name,
    description: server.description ?? "",
    mode: server.execution_mode,
    transport: server.transport,
    command: config?.command ?? "",
    args: (config?.args ?? []).join(" "),
    url: config?.url ?? "",
    packageName: config?.package_name ?? "",
    packageVersion: config?.package_version ?? "",
    configuration: cloneConfiguration(config),
    secretBindings: [...(server.secret_bindings ?? [])],
  };
}

function toConfiguration(draft: FormDraft): MCPServerConfiguration {
  if (draft.mode === "remote") return { ...draft.configuration, url: draft.url.trim() };
  if (draft.mode === "managed_package") {
    return {
      ...draft.configuration,
      package_type: "npm",
      package_name: draft.packageName.trim(),
      package_version: draft.packageVersion.trim(),
    };
  }
  return {
    ...draft.configuration,
    command: draft.command.trim(),
    args: draft.args.trim() ? draft.args.trim().split(/\s+/) : [],
  };
}

function toPayload(draft: FormDraft): MCPDefinitionInput {
  return {
    runtime_name: draft.runtimeName.trim(),
    display_name: draft.displayName.trim(),
    description: draft.description.trim(),
    execution_mode: draft.mode,
    transport: draft.mode === "remote" ? draft.transport : "stdio",
    configuration: toConfiguration(draft),
    secret_bindings: draft.secretBindings,
    source: "custom",
  };
}

function validateDraft(draft: FormDraft, t: (key: string) => string): string | null {
  if (!draft.runtimeName.trim() || !draft.displayName.trim()) {
    return t("settings:mcpDefinitionNameRequired");
  }
  if (draft.mode === "remote" && !/^https?:\/\/[^\s]+$/i.test(draft.url.trim())) {
    return t("settings:mcpRemoteUrlRequired");
  }
  if (
    draft.mode === "managed_package" &&
    (!draft.packageName.trim() || !draft.packageVersion.trim())
  ) {
    return t("settings:mcpPackageVersionRequired");
  }
  if (draft.mode === "existing_executable" && !draft.command.trim()) {
    return t("settings:mcpCommandRequired");
  }
  return null;
}

function transportForMode(mode: MCPExecutionMode, current: MCPTransport): MCPTransport {
  if (mode !== "remote") return "stdio";
  if (current === "sse" || current === "streamable_http") return current;
  return "streamable_http";
}

function FormField({
  id,
  label,
  value,
  onChange,
  placeholder,
  description,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  description?: string;
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
    </div>
  );
}

function ModeFields({
  draft,
  setDraft,
}: {
  draft: FormDraft;
  setDraft: (next: Partial<FormDraft>) => void;
}) {
  const { t } = useTranslation();
  if (draft.mode === "remote") {
    return (
      <>
        <div className="space-y-2">
          <Label htmlFor="mcp-transport">{t("settings:mcpTransport")}</Label>
          <Select
            value={draft.transport}
            onValueChange={(transport) => setDraft({ transport: transport as MCPTransport })}
          >
            <SelectTrigger id="mcp-transport" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="streamable_http">
                {t("settings:mcpTransportStreamable")}
              </SelectItem>
              <SelectItem value="sse">{t("settings:mcpTransportSse")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <FormField
          id="mcp-url"
          label={t("settings:mcpRemoteUrl")}
          value={draft.url}
          onChange={(url) => setDraft({ url })}
          description={t("settings:mcpRemoteUrlHelp")}
        />
      </>
    );
  }
  if (draft.mode === "managed_package") {
    return (
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          id="mcp-package-name"
          label={t("settings:mcpPackageName")}
          value={draft.packageName}
          onChange={(packageName) => setDraft({ packageName })}
          placeholder={MCP_RUNTIME_NAME_PLACEHOLDER}
        />
        <FormField
          id="mcp-package-version"
          label={t("settings:mcpPackageVersion")}
          value={draft.packageVersion}
          onChange={(packageVersion) => setDraft({ packageVersion })}
          placeholder="1.2.3"
          description={t("settings:mcpPackageVersionHelp")}
        />
      </div>
    );
  }
  return (
    <>
      <FormField
        id="mcp-command"
        label={t("settings:mcpExecutable")}
        value={draft.command}
        onChange={(command) => setDraft({ command })}
        placeholder="node"
        description={t("settings:mcpExecutableHelp")}
      />
      <FormField
        id="mcp-args"
        label={t("settings:mcpArguments")}
        value={draft.args}
        onChange={(args) => setDraft({ args })}
        placeholder="server.js"
        description={t("settings:mcpArgumentsHelp")}
      />
    </>
  );
}

function MCPDefinitionFormHeader({
  server,
  saving,
  onClose,
}: Pick<MCPDefinitionFormProps, "server" | "onClose"> & { saving: boolean }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 border-b p-4">
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-11 w-11 cursor-pointer md:h-8 md:w-8"
        onClick={onClose}
        aria-label={t("settings:mcpBackToList")}
      >
        <IconArrowLeft className="size-4" />
      </Button>
      <div className="min-w-0">
        <h3 className="text-base font-semibold">
          {server ? t("settings:editMcpServer") : t("settings:addMcpServer")}
        </h3>
        <p className="text-xs text-muted-foreground">{t("settings:mcpSaveWithSharedControl")}</p>
      </div>
      {saving && (
        <IconDeviceFloppy
          className="ml-auto size-4 animate-pulse"
          aria-label={t("settings:saving")}
        />
      )}
    </div>
  );
}

function MCPDefinitionFormBody({
  draft,
  setDraft,
  error,
}: {
  draft: FormDraft;
  setDraft: (next: Partial<FormDraft>) => void;
  error: string | null;
}) {
  const { t } = useTranslation();
  const setMode = (mode: MCPExecutionMode) =>
    setDraft({
      mode,
      transport: transportForMode(mode, draft.transport),
    });
  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          id="mcp-runtime-name"
          label={t("settings:mcpRuntimeName")}
          value={draft.runtimeName}
          onChange={(runtimeName) => setDraft({ runtimeName })}
          description={t("settings:mcpRuntimeNameHelp")}
        />
        <FormField
          id="mcp-display-name"
          label={t("settings:mcpDisplayName")}
          value={draft.displayName}
          onChange={(displayName) => setDraft({ displayName })}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="mcp-description">{t("settings:mcpDescription")}</Label>
        <Textarea
          id="mcp-description"
          value={draft.description}
          onChange={(event) => setDraft({ description: event.target.value })}
          className="min-h-20"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="mcp-execution-mode">{t("settings:mcpSetupMode")}</Label>
        <Select value={draft.mode} onValueChange={(mode) => setMode(mode as MCPExecutionMode)}>
          <SelectTrigger id="mcp-execution-mode" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="remote">{t("settings:mcpModeRemote")}</SelectItem>
            <SelectItem value="managed_package">{t("settings:mcpModeManagedPackage")}</SelectItem>
            <SelectItem value="existing_executable">
              {t("settings:mcpModeExistingExecutable")}
            </SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">{t(`settings:mcpModeHelp_${draft.mode}`)}</p>
      </div>
      <ModeFields draft={draft} setDraft={setDraft} />
      {error && (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

export function MCPDefinitionForm({
  workspaceId,
  server,
  onSave,
  onClose,
}: MCPDefinitionFormProps) {
  const { t } = useTranslation();
  const initial = useMemo(() => draftFromServer(server), [server]);
  const [draft, setDraftState] = useState<FormDraft>(initial);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const isDirty = JSON.stringify(draft) !== JSON.stringify(initial);
  const validationError = validateDraft(draft, t);
  const setDraft = (next: Partial<FormDraft>) =>
    setDraftState((current) => ({ ...current, ...next }));

  useEffect(() => setDraftState(initial), [initial]);

  const save = async () => {
    if (validationError) {
      setError(validationError);
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await onSave(toPayload(draft), server ?? undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("settings:mcpSaveFailed"));
      throw cause;
    } finally {
      setSaving(false);
    }
  };

  useSettingsSaveContributor({
    id: `workspace-mcp-definition:${workspaceId}:${server?.id ?? "new"}`,
    revision: JSON.stringify(draft),
    isDirty,
    canSave: !validationError && !saving,
    invalidReason: validationError ?? undefined,
    save,
    discard: () => {
      setDraftState(initial);
      setError(null);
      onClose();
    },
  });

  return (
    <section
      className="flex min-h-[calc(100dvh-15rem)] min-w-0 flex-col rounded-lg border bg-card"
      data-testid="mcp-definition-form"
    >
      <MCPDefinitionFormHeader server={server} saving={saving} onClose={onClose} />
      <MCPDefinitionFormBody draft={draft} setDraft={setDraft} error={error} />
    </section>
  );
}
