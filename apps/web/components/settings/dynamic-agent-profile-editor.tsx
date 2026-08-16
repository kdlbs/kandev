"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowDown, IconArrowUp, IconInfoCircle, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { AgentLogo } from "@/components/agent-logo";
import { AgentProfilePicker } from "@/components/settings/agent-profile-picker";
import { ProfileNameField } from "@/components/settings/profile-form-fields";
import { ProfileEnabledHelp } from "@/components/settings/profile-enabled-help";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { updateAgentProfileAction } from "@/app/actions/agents";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type { DynamicAgentCandidate } from "@/lib/types/agent-profile";

type DynamicAgentProfileEditorProps = {
  agent: Agent;
  profile: AgentProfile;
  /** When supplied, render as a draft editor owned by the parent save flow. */
  onDraftChange?: (patch: Pick<AgentProfile, "name" | "dynamic" | "enabled">) => void;
};

const defaultProviderErrorAction = "try_next";

function dynamicDraftRevision(
  name: string,
  candidates: DynamicAgentCandidate[],
  enabled: boolean,
): string {
  return JSON.stringify({ name, candidates, enabled });
}

function candidateLabel(candidate: DynamicAgentCandidate, profiles: AgentProfile[]): string {
  return (
    profiles.find((profile) => profile.id === candidate.executionProfileId)?.name ??
    candidate.executionProfileId
  );
}

function dynamicRouteActionCopy(action: string): {
  labelKey: "agents:retry" | "task:stop" | "task:dynamicRouteTryNext";
  descriptionKey:
    | "agents:dynamicRouteRetryDescription"
    | "agents:dynamicRouteStopDescription"
    | "agents:dynamicRouteTryNextDescription";
} {
  switch (action) {
    case "retry_same":
      return {
        labelKey: "agents:retry",
        descriptionKey: "agents:dynamicRouteRetryDescription",
      };
    case "stop":
      return {
        labelKey: "task:stop",
        descriptionKey: "agents:dynamicRouteStopDescription",
      };
    default:
      return {
        labelKey: "task:dynamicRouteTryNext",
        descriptionKey: "agents:dynamicRouteTryNextDescription",
      };
  }
}

function DynamicRouteActionHelp({ action }: { action: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { labelKey, descriptionKey } = dynamicRouteActionCopy(action);
  const actionLabel = t(labelKey);

  return (
    <Tooltip open={open} onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="size-11 shrink-0 cursor-help text-muted-foreground sm:size-7"
          aria-label={t("agents:dynamicRouteActionInfo", { action: actionLabel })}
          onClick={() => setOpen((current) => !current)}
          data-testid={`dynamic-route-action-help-${action}`}
        >
          <IconInfoCircle className="size-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs text-xs leading-relaxed">
        {t(descriptionKey)}
      </TooltipContent>
    </Tooltip>
  );
}

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

// eslint-disable-next-line max-lines-per-function, complexity -- coordinates draft, save, and candidate state.
export function DynamicAgentProfileEditor({
  agent,
  profile,
  onDraftChange,
}: DynamicAgentProfileEditorProps) {
  const { t } = useTranslation();
  const enabledLabel = t("agents:enabled");
  const { toast } = useToast();
  const routingEnabled = useFeature("dynamicAgentRouting");
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const [name, setName] = useState(profile.name);
  const [candidates, setCandidates] = useState<DynamicAgentCandidate[]>(
    profile.dynamic?.candidates ?? [],
  );
  const [profileEnabled, setProfileEnabled] = useState(profile.enabled !== false);
  const [dynamicVersion, setDynamicVersion] = useState(profile.dynamic?.version ?? 1);
  const initialCandidates = profile.dynamic?.candidates ?? [];
  const initialRevision = dynamicDraftRevision(
    profile.name,
    initialCandidates,
    profile.enabled !== false,
  );
  const [savedRevision, setSavedRevision] = useState(initialRevision);
  const [saving, setSaving] = useState(false);
  const standalone = onDraftChange === undefined;

  const concreteProfiles = useMemo(
    () =>
      settingsAgents
        .filter((item) => item.name !== "dynamic")
        .flatMap((item) => item.profiles)
        .filter(
          (candidate) =>
            candidate.kind !== "dynamic" && candidate.enabled !== false && !candidate.workspaceId,
        ),
    [settingsAgents],
  );
  const availableProfileOptions = useMemo(
    () =>
      settingsAgents.flatMap((item) =>
        item.name === "dynamic"
          ? []
          : item.profiles
              .filter(
                (candidate) =>
                  candidate.kind !== "dynamic" &&
                  candidate.enabled !== false &&
                  !candidate.workspaceId &&
                  !candidates.some((item) => item.executionProfileId === candidate.id),
              )
              .map((candidate) => toAgentProfileOption(item, candidate)),
      ),
    [candidates, settingsAgents],
  );

  const notifyDraft = (
    nextName: string,
    nextCandidates: DynamicAgentCandidate[],
    nextEnabled = profileEnabled,
  ) => {
    onDraftChange?.({
      name: nextName,
      enabled: nextEnabled,
      dynamic: {
        version: dynamicVersion,
        candidates: nextCandidates,
      },
    });
  };

  const addCandidate = (executionProfileId: string) => {
    setCandidates((current) => {
      const next = [
        ...current,
        {
          position: current.length,
          executionProfileId: toAgentProfileId(executionProfileId),
          enabled: true,
          rules: { on_provider_error: defaultProviderErrorAction },
        },
      ];
      notifyDraft(name, next);
      return next;
    });
  };

  const moveCandidate = (index: number, direction: -1 | 1) => {
    setCandidates((current) => {
      const target = index + direction;
      if (target < 0 || target >= current.length) return current;
      const next = [...current];
      [next[index], next[target]] = [next[target], next[index]];
      const reordered = next.map((candidate, position) => ({ ...candidate, position }));
      notifyDraft(name, reordered);
      return reordered;
    });
  };

  const removeCandidate = (index: number) => {
    setCandidates((current) => {
      const next = current
        .filter((_, candidateIndex) => candidateIndex !== index)
        .map((candidate, position) => ({
          ...candidate,
          position,
        }));
      notifyDraft(name, next);
      return next;
    });
  };

  const updateCandidate = (index: number, patch: Partial<DynamicAgentCandidate>) => {
    setCandidates((current) => {
      const next = current.map((candidate, candidateIndex) =>
        candidateIndex === index ? { ...candidate, ...patch } : candidate,
      );
      notifyDraft(name, next);
      return next;
    });
  };

  const updateCandidateAction = (index: number, action: string) => {
    setCandidates((current) => {
      const next = current.map((candidate, candidateIndex) =>
        candidateIndex === index
          ? { ...candidate, rules: { ...candidate.rules, on_provider_error: action } }
          : candidate,
      );
      notifyDraft(name, next);
      return next;
    });
  };

  const save = async () => {
    if (!routingEnabled || !name.trim() || candidates.length === 0 || !profile.dynamic) return;
    setSaving(true);
    try {
      const draftPayload = {
        name: name.trim(),
        enabled: profileEnabled,
        dynamic: {
          version: dynamicVersion,
          candidates,
        },
      };
      const payload = {
        name: name.trim(),
        enabled: profileEnabled,
        dynamic: {
          version: dynamicVersion,
          candidates: candidates.map((candidate, position) => ({
            position,
            execution_profile_id: candidate.executionProfileId,
            enabled: candidate.enabled,
            rules: candidate.rules,
          })),
        },
      };
      if (onDraftChange) {
        onDraftChange(draftPayload);
        return;
      }
      const updated = await updateAgentProfileAction(profile.id, payload);
      const nextAgents = settingsAgents.map((item) =>
        item.id !== agent.id
          ? item
          : {
              ...item,
              profiles: item.profiles.map((itemProfile) =>
                itemProfile.id === updated.id ? updated : itemProfile,
              ),
            },
      );
      setSettingsAgents(nextAgents);
      setAgentProfiles(
        nextAgents.flatMap((item) =>
          item.profiles.map((itemProfile) => toAgentProfileOption(item, itemProfile)),
        ),
      );
      setName(updated.name);
      setProfileEnabled(updated.enabled !== false);
      const nextCandidates = updated.dynamic?.candidates ?? candidates;
      setCandidates(nextCandidates);
      setDynamicVersion(updated.dynamic?.version ?? dynamicVersion + 1);
      setSavedRevision(
        dynamicDraftRevision(updated.name, nextCandidates, updated.enabled !== false),
      );
      toast({ title: t("agents:dynamicProfileSaved") });
    } catch (error) {
      toast({
        title: t("agents:failedToSaveProfile"),
        description: error instanceof Error ? error.message : undefined,
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  const draftRevision = dynamicDraftRevision(name, candidates, profileEnabled);
  useSettingsSaveContributor({
    id: `dynamic-profile:${profile.id}`,
    revision: draftRevision,
    isDirty: standalone && draftRevision !== savedRevision,
    canSave: routingEnabled && !saving && Boolean(name.trim()) && candidates.length > 0,
    invalidReason: !name.trim() ? t("agents:profileNameRequired") : t("agents:noDynamicCandidates"),
    save,
    discard: () => {
      if (!standalone) return;
      setName(profile.name);
      setProfileEnabled(profile.enabled !== false);
      setCandidates(initialCandidates);
      setDynamicVersion(profile.dynamic?.version ?? 1);
      setSavedRevision(initialRevision);
    },
  });

  if (!routingEnabled) {
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
        value={name}
        onChange={(nextName) => {
          setName(nextName);
          notifyDraft(nextName, candidates);
        }}
      />

      <DynamicRoutingPolicyHelp />

      <div className="space-y-3" data-testid="dynamic-profile-candidates">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h3 className="text-sm font-medium">{t("agents:dynamicCandidates")}</h3>
            <p className="text-xs text-muted-foreground">
              {t("agents:dynamicCandidatesDescription")}
            </p>
          </div>
          <AgentProfilePicker
            profiles={availableProfileOptions}
            value=""
            onValueChange={(value) => {
              if (value) addCandidate(value);
            }}
            testId="add-dynamic-candidate"
            placeholder={t("agents:addDynamicCandidate")}
            searchPlaceholder={t("agents:searchDynamicCandidates")}
            emptyMessage={t("agents:noDynamicCandidatesFound")}
            ariaLabel={t("agents:addDynamicCandidate")}
            triggerClassName="min-h-11 w-full sm:w-auto"
          />
        </div>

        {candidates.length === 0 ? (
          <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
            {t("agents:noDynamicCandidates")}
          </p>
        ) : (
          <ol className="grid gap-2">
            {candidates.map((candidate, index) => (
              <li
                key={candidate.executionProfileId}
                className="flex min-w-0 flex-col gap-2 rounded-md border p-2 sm:flex-row sm:items-center"
              >
                <Badge variant="outline">{index + 1}</Badge>
                <span className="min-w-0 flex-1 truncate text-sm">
                  {candidateLabel(candidate, concreteProfiles)}
                </span>
                <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:shrink-0">
                  <div className="flex min-h-11 items-center gap-2 rounded-md border px-2">
                    <span className="text-xs text-muted-foreground">{enabledLabel}</span>
                    <Switch
                      checked={candidate.enabled}
                      onCheckedChange={(checked) => updateCandidate(index, { enabled: checked })}
                      aria-label={`${enabledLabel}: ${candidateLabel(candidate, concreteProfiles)}`}
                    />
                  </div>
                  <div className="flex min-h-11 w-full items-center gap-1 sm:w-auto">
                    <Select
                      value={candidate.rules?.on_provider_error ?? defaultProviderErrorAction}
                      onValueChange={(action) => updateCandidateAction(index, action)}
                    >
                      <SelectTrigger
                        className="min-h-11 min-w-0 flex-1 sm:w-40 sm:flex-none"
                        aria-label={t("agents:dynamicRouteAction")}
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="retry_same">{t("agents:retry")}</SelectItem>
                        <SelectItem value={defaultProviderErrorAction}>
                          {t("task:dynamicRouteTryNext")}
                        </SelectItem>
                        <SelectItem value="stop">{t("task:stop")}</SelectItem>
                      </SelectContent>
                    </Select>
                    <DynamicRouteActionHelp
                      action={candidate.rules?.on_provider_error ?? defaultProviderErrorAction}
                    />
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  className="min-h-11 min-w-11 shrink-0 cursor-pointer"
                  onClick={() => moveCandidate(index, -1)}
                  disabled={index === 0}
                  aria-label={t("agents:moveDynamicCandidateUp")}
                >
                  <IconArrowUp className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="min-h-11 min-w-11 shrink-0 cursor-pointer"
                  onClick={() => moveCandidate(index, 1)}
                  disabled={index === candidates.length - 1}
                  aria-label={t("agents:moveDynamicCandidateDown")}
                >
                  <IconArrowDown className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="min-h-11 min-w-11 shrink-0 cursor-pointer text-destructive"
                  onClick={() => removeCandidate(index)}
                  aria-label={t("agents:removeDynamicCandidate")}
                >
                  <IconTrash className="h-4 w-4" />
                </Button>
              </li>
            ))}
          </ol>
        )}
      </div>
    </>
  );

  return (
    <div className="min-w-0 space-y-6 overflow-x-hidden">
      {standalone && (
        <DynamicProfileEditorHeader
          agent={agent}
          profile={profile}
          name={name}
          enabled={profileEnabled}
          onEnabledChange={(checked) => {
            setProfileEnabled(checked);
            notifyDraft(name, candidates, checked);
          }}
        />
      )}

      {standalone ? (
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
