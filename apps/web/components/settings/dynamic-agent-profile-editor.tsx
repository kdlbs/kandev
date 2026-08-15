"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowDown, IconArrowUp, IconPlus, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { AgentLogo } from "@/components/agent-logo";
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

// eslint-disable-next-line max-lines-per-function -- coordinates the shared desktop editor and mobile picker state.
export function DynamicAgentProfileEditor({
  agent,
  profile,
  onDraftChange,
}: DynamicAgentProfileEditorProps) {
  const { t } = useTranslation();
  const enabledLabel = t("agents:enabled");
  const { toast } = useToast();
  const enabled = useFeature("dynamicAgentRouting");
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
  const [pickerOpen, setPickerOpen] = useState(false);
  const [saving, setSaving] = useState(false);

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
  const availableProfiles = concreteProfiles.filter(
    (candidate) => !candidates.some((item) => item.executionProfileId === candidate.id),
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
    setPickerOpen(false);
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
    if (!enabled || !name.trim() || candidates.length === 0 || !profile.dynamic) return;
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

  const standalone = onDraftChange === undefined;
  const draftRevision = dynamicDraftRevision(name, candidates, profileEnabled);
  useSettingsSaveContributor({
    id: `dynamic-profile:${profile.id}`,
    revision: draftRevision,
    isDirty: standalone && draftRevision !== savedRevision,
    canSave: enabled && !saving && Boolean(name.trim()) && candidates.length > 0,
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

  if (!enabled) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-sm text-muted-foreground">{t("agents:dynamicProfileDisabled")}</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="min-w-0 space-y-6 overflow-x-hidden">
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
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("agents:dynamicProfileSettings")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="flex min-h-11 items-center justify-between gap-3 rounded-md border p-3">
            <div className="min-w-0">
              <p className="text-sm font-medium">{enabledLabel}</p>
              <p className="text-xs text-muted-foreground">{t("agents:enabledProfileHelper")}</p>
            </div>
            <Switch
              id={`dynamic-profile-enabled-${profile.id}`}
              checked={profileEnabled}
              onCheckedChange={(checked) => {
                setProfileEnabled(checked);
                notifyDraft(name, candidates, checked);
              }}
              aria-label={enabledLabel}
            />
          </div>
          <label className="grid gap-2 text-sm font-medium" htmlFor="dynamic-profile-name">
            {t("agents:profileName")}
            <Input
              id="dynamic-profile-name"
              value={name}
              onChange={(event) => {
                const nextName = event.target.value;
                setName(nextName);
                notifyDraft(nextName, candidates);
              }}
              className="min-h-11"
              data-testid="dynamic-profile-name"
            />
          </label>

          <div className="space-y-3" data-testid="dynamic-profile-candidates">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <h3 className="text-sm font-medium">{t("agents:dynamicCandidates")}</h3>
                <p className="text-xs text-muted-foreground">
                  {t("agents:dynamicCandidatesDescription")}
                </p>
              </div>
              <Button
                variant="outline"
                className="min-h-11"
                onClick={() => setPickerOpen(true)}
                disabled={availableProfiles.length === 0}
                data-testid="add-dynamic-candidate"
              >
                <IconPlus className="mr-2 h-4 w-4" />
                {t("agents:addDynamicCandidate")}
              </Button>
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
                          onCheckedChange={(checked) =>
                            updateCandidate(index, { enabled: checked })
                          }
                          aria-label={`${enabledLabel}: ${candidateLabel(candidate, concreteProfiles)}`}
                        />
                      </div>
                      <Select
                        value={candidate.rules?.on_provider_error ?? defaultProviderErrorAction}
                        onValueChange={(action) => updateCandidateAction(index, action)}
                      >
                        <SelectTrigger
                          className="min-h-11 w-full sm:w-40"
                          aria-label={t("agents:retry")}
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
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="min-h-11 min-w-11 shrink-0"
                      onClick={() => moveCandidate(index, -1)}
                      disabled={index === 0}
                      aria-label={t("agents:moveDynamicCandidateUp")}
                    >
                      <IconArrowUp className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="min-h-11 min-w-11 shrink-0"
                      onClick={() => moveCandidate(index, 1)}
                      disabled={index === candidates.length - 1}
                      aria-label={t("agents:moveDynamicCandidateDown")}
                    >
                      <IconArrowDown className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="min-h-11 min-w-11 shrink-0 text-destructive"
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
        </CardContent>
      </Card>

      <MobilePickerSheet
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        title={t("agents:addDynamicCandidate")}
        description={t("agents:dynamicCandidatePickerDescription")}
      >
        <div className="grid gap-2 pb-2" data-testid="dynamic-candidate-picker">
          {availableProfiles.map((candidate) => (
            <Button
              key={candidate.id}
              variant="ghost"
              className="min-h-11 justify-start"
              onClick={() => addCandidate(candidate.id)}
            >
              {candidate.agentDisplayName} • {candidate.name}
            </Button>
          ))}
        </div>
      </MobilePickerSheet>
    </div>
  );
}
