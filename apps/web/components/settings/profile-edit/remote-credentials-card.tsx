"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLoader2 } from "@tabler/icons-react";
import { CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Accordion } from "@kandev/ui/accordion";
import { SettingsCard } from "@/components/settings/settings-card";
import {
  GitIdentityAccordionItem,
  type GitIdentityMode,
  type GitIdentityState,
} from "./git-identity-fields";
import { listRemoteCredentials, type RemoteAuthSpec } from "@/lib/api/domains/settings-api";
import { listAgentConfigBundles, type AgentConfigBundle } from "@/lib/api/domains/agent-config-api";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { AuthSection } from "./remote-auth-section";
export type { GitIdentityMode, GitIdentityState } from "./git-identity-fields";

type RemoteCredentialsCardProps = {
  isRemote: boolean;
  selectedIds: string[];
  baselineSelectedIds?: string[];
  onChange: (ids: string[]) => void;
  configBundleIds: string[];
  baselineConfigBundleIds?: string[];
  onConfigBundleChange: (ids: string[]) => void;
  isSSH?: boolean;
  agentEnvVars: Record<string, string | null>;
  baselineAgentEnvVars?: Record<string, string | null>;
  onAgentEnvVarChange: (methodId: string, secretId: string | null) => void;
  secrets: SecretListItem[];
  gitIdentityMode: GitIdentityMode;
  baselineGitIdentityMode?: GitIdentityMode;
  onGitIdentityModeChange: (mode: GitIdentityMode) => void;
  gitUserName: string;
  gitUserEmail: string;
  baselineGitUserName?: string;
  baselineGitUserEmail?: string;
  onGitUserNameChange: (value: string) => void;
  onGitUserEmailChange: (value: string) => void;
  localGitIdentity: GitIdentityState;
};

export function RemoteCredentialsCard({
  isRemote,
  selectedIds,
  baselineSelectedIds = [],
  onChange,
  configBundleIds,
  baselineConfigBundleIds = [],
  onConfigBundleChange,
  isSSH = false,
  agentEnvVars,
  baselineAgentEnvVars = {},
  onAgentEnvVarChange,
  secrets,
  gitIdentityMode,
  baselineGitIdentityMode = "override",
  onGitIdentityModeChange,
  gitUserName,
  gitUserEmail,
  baselineGitUserName = "",
  baselineGitUserEmail = "",
  onGitUserNameChange,
  onGitUserEmailChange,
  localGitIdentity,
}: RemoteCredentialsCardProps) {
  const { authSpecs, configBundles, loading } = useRemoteCredentialCatalog();
  const isDirty = hasRemoteCredentialsChanges({
    selectedIds,
    baselineSelectedIds,
    configBundleIds,
    baselineConfigBundleIds,
    agentEnvVars,
    baselineAgentEnvVars,
    gitIdentityMode,
    baselineGitIdentityMode,
    gitUserName,
    gitUserEmail,
    baselineGitUserName,
    baselineGitUserEmail,
  });
  const selectedSet = new Set(selectedIds);
  const baselineSelectedSet = new Set(baselineSelectedIds);
  const agentSections = mergeAgentSections(authSpecs, configBundles);

  if (loading) {
    return <RemoteCredentialsLoading isDirty={isDirty} />;
  }

  return (
    <SettingsCard isDirty={isDirty}>
      <RemoteCredentialsHeader />
      <CardContent className="space-y-4">
        {agentSections.length > 0 || isRemote ? (
          <>
            <Accordion type="multiple">
              {isRemote && (
                <GitIdentityAccordionItem
                  mode={gitIdentityMode}
                  baselineMode={baselineGitIdentityMode}
                  onModeChange={onGitIdentityModeChange}
                  gitUserName={gitUserName}
                  gitUserEmail={gitUserEmail}
                  baselineGitUserName={baselineGitUserName}
                  baselineGitUserEmail={baselineGitUserEmail}
                  onGitUserNameChange={onGitUserNameChange}
                  onGitUserEmailChange={onGitUserEmailChange}
                  localGitIdentity={localGitIdentity}
                />
              )}
              {agentSections.map(({ spec, configBundles: agentConfigBundles }) => (
                <AuthSection
                  key={spec.id}
                  spec={spec}
                  selectedIds={selectedSet}
                  baselineSelectedIds={baselineSelectedSet}
                  onCredentialsChange={onChange}
                  agentEnvVars={agentEnvVars}
                  baselineAgentEnvVars={baselineAgentEnvVars}
                  onMethodSecretChange={onAgentEnvVarChange}
                  secrets={secrets}
                  configBundles={agentConfigBundles}
                  configBundleIds={configBundleIds}
                  baselineConfigBundleIds={baselineConfigBundleIds}
                  onConfigBundleChange={onConfigBundleChange}
                  isSSH={isSSH}
                />
              ))}
            </Accordion>
          </>
        ) : (
          <NoTransferableCredentials />
        )}
      </CardContent>
    </SettingsCard>
  );
}

function RemoteCredentialsHeader() {
  const { t } = useTranslation();
  return (
    <CardHeader>
      <CardTitle>{t("executors:remoteCredentials")}</CardTitle>
      <CardDescription>{t("executors:configureAuthenticationForToolsAndAgents")}</CardDescription>
    </CardHeader>
  );
}

function NoTransferableCredentials() {
  const { t } = useTranslation();
  return (
    <p className="text-sm text-muted-foreground">{t("executors:noTransferableCredentialsFound")}</p>
  );
}

function RemoteCredentialsLoading({ isDirty }: { isDirty: boolean }) {
  const { t } = useTranslation();
  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle>{t("executors:remoteCredentials")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <IconLoader2 className="h-4 w-4 animate-spin" />
          {t("executors:loading")}
        </div>
      </CardContent>
    </SettingsCard>
  );
}

function useRemoteCredentialCatalog() {
  const [authSpecs, setAuthSpecs] = useState<RemoteAuthSpec[]>([]);
  const [configBundles, setConfigBundles] = useState<AgentConfigBundle[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    void Promise.allSettled([listRemoteCredentials(), listAgentConfigBundles()]).then(
      ([authResult, configResult]) => {
        if (authResult.status === "fulfilled") setAuthSpecs(authResult.value.auth_specs ?? []);
        if (configResult.status === "fulfilled") setConfigBundles(configResult.value.bundles ?? []);
        setLoading(false);
      },
    );
  }, []);

  return { authSpecs, configBundles, loading };
}

type RemoteAgentSection = {
  spec: RemoteAuthSpec;
  configBundles: AgentConfigBundle[];
};

function mergeAgentSections(
  authSpecs: RemoteAuthSpec[],
  configBundles: AgentConfigBundle[],
): RemoteAgentSection[] {
  const sections = new Map<string, RemoteAgentSection>();

  for (const spec of authSpecs) {
    sections.set(spec.id, { spec, configBundles: [] });
  }

  for (const bundle of configBundles) {
    const section = sections.get(bundle.agent_id);
    if (section) {
      section.configBundles.push(bundle);
      continue;
    }

    sections.set(bundle.agent_id, {
      spec: {
        id: bundle.agent_id,
        display_name: bundle.display_name,
        methods: [],
      },
      configBundles: [bundle],
    });
  }

  return [...sections.values()];
}

function hasRemoteCredentialsChanges({
  selectedIds,
  baselineSelectedIds,
  configBundleIds,
  baselineConfigBundleIds,
  agentEnvVars,
  baselineAgentEnvVars,
  gitIdentityMode,
  baselineGitIdentityMode,
  gitUserName,
  gitUserEmail,
  baselineGitUserName,
  baselineGitUserEmail,
}: Pick<
  RemoteCredentialsCardProps,
  | "selectedIds"
  | "baselineSelectedIds"
  | "configBundleIds"
  | "baselineConfigBundleIds"
  | "agentEnvVars"
  | "baselineAgentEnvVars"
  | "gitIdentityMode"
  | "baselineGitIdentityMode"
  | "gitUserName"
  | "gitUserEmail"
  | "baselineGitUserName"
  | "baselineGitUserEmail"
>): boolean {
  const credentialsDirty = !sameStringSet(new Set(selectedIds), new Set(baselineSelectedIds));
  const configBundlesDirty = !sameStringSet(
    new Set(configBundleIds),
    new Set(baselineConfigBundleIds),
  );
  const agentEnvVarsDirty = JSON.stringify(agentEnvVars) !== JSON.stringify(baselineAgentEnvVars);
  const gitIdentityDirty =
    gitIdentityMode !== baselineGitIdentityMode ||
    (gitIdentityMode === "override" &&
      (gitUserName !== baselineGitUserName || gitUserEmail !== baselineGitUserEmail));
  return credentialsDirty || configBundlesDirty || agentEnvVarsDirty || gitIdentityDirty;
}

function sameStringSet(left: Set<string>, right: Set<string>): boolean {
  return left.size === right.size && [...left].every((value) => right.has(value));
}
