"use client";

import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import { AgentLogo } from "@/components/agent-logo";
import { DynamicAgentCandidateList } from "@/components/settings/dynamic-agent-candidate-list";
import { ProfileEnabledHelp } from "@/components/settings/profile-enabled-help";
import { ProfileNameField } from "@/components/settings/profile-form-fields";
import { useDynamicAgentProfileEditorState } from "@/components/settings/dynamic-agent-profile-editor-state";
import type { Agent, AgentProfile } from "@/lib/types/http";

type DynamicAgentProfileEditorProps = {
  agent: Agent;
  profile: AgentProfile;
  isCreating?: boolean;
  /** When supplied, render as a draft editor owned by the parent save flow. */
  onDraftChange?: (patch: Pick<AgentProfile, "name" | "dynamic" | "enabled">) => void;
};

function DynamicRoutingPolicyHelp() {
  const { t } = useTranslation();
  return (
    <div
      className="flex gap-3 rounded-md border bg-muted/30 p-3"
      data-testid="dynamic-routing-policy-help"
    >
      <IconInfoCircle className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
      <div className="min-w-0 space-y-1 text-xs leading-relaxed text-muted-foreground">
        <p className="font-medium text-foreground">{t("agents:dynamicRoutingPolicyTitle")}</p>
        <p>{t("agents:dynamicRoutingPolicySwitchable")}</p>
        <p>{t("agents:dynamicRoutingPolicyExcluded")}</p>
        <p>{t("agents:dynamicRoutingPolicyRecovery")}</p>
        <p>{t("agents:dynamicPolicyWaitExplanation")}</p>
      </div>
    </div>
  );
}

function DynamicProfileEditorHeader({
  agent,
  profile,
  name,
  enabled,
  onEnabledChange,
}: {
  agent: Agent;
  profile: AgentProfile;
  name: string;
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex flex-col items-stretch justify-between gap-4 sm:flex-row sm:items-start">
        <div className="min-w-0">
          <h2 className="flex min-w-0 items-center gap-2 text-2xl font-bold wrap-break-word">
            <AgentLogo agentName={agent.name} size={28} className="shrink-0" />
            {profile.agentDisplayName} • {name}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("agents:dynamicProfileDescription")}
          </p>
        </div>
        <div className="flex items-center gap-3 sm:shrink-0">
          <div className="flex items-center gap-1 text-left sm:text-right">
            <p className="text-sm font-medium">{t("agents:enabled")}</p>
            <ProfileEnabledHelp />
          </div>
          <Switch
            checked={enabled}
            onCheckedChange={onEnabledChange}
            data-testid="dynamic-profile-enabled-toggle"
            aria-label={enabled ? t("agents:disableProfile") : t("agents:enableProfile")}
          />
        </div>
      </div>
      <Separator />
    </>
  );
}

export function DynamicAgentProfileEditor({
  agent,
  profile,
  isCreating = false,
  onDraftChange,
}: DynamicAgentProfileEditorProps) {
  const { t } = useTranslation();
  const state = useDynamicAgentProfileEditorState({ agent, profile, onDraftChange });

  if (!state.routingEnabled) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-sm text-muted-foreground">{t("agents:dynamicProfileDisabled")}</p>
        </CardContent>
      </Card>
    );
  }

  const editorContent = (
    <>
      <ProfileNameField
        id="dynamic-profile-name"
        testId="dynamic-profile-name"
        value={state.name}
        onChange={state.updateName}
      />
      <DynamicRoutingPolicyHelp />
      <DynamicAgentCandidateList
        candidates={state.candidates}
        concreteProfiles={state.concreteProfiles}
        availableProfileOptions={state.availableProfileOptions}
        enabledLabel={state.enabledLabel}
        addCandidate={state.addCandidate}
        moveCandidate={state.moveCandidate}
        removeCandidate={state.removeCandidate}
        updateCandidate={state.updateCandidate}
        updateCandidatePolicy={state.updateCandidatePolicy}
      />
    </>
  );

  return (
    <div className="min-w-0 space-y-6 overflow-x-hidden">
      {state.standalone && !isCreating && (
        <DynamicProfileEditorHeader
          agent={agent}
          profile={profile}
          name={state.name}
          enabled={state.profileEnabled}
          onEnabledChange={state.updateProfileEnabled}
        />
      )}
      {state.standalone ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("agents:dynamicProfileSettings")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-5">{editorContent}</CardContent>
        </Card>
      ) : (
        <div className="space-y-5">{editorContent}</div>
      )}
    </div>
  );
}
