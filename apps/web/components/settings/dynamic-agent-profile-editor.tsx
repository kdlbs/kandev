"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowDown, IconArrowUp, IconPlus, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
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
  onDraftChange?: (patch: Pick<AgentProfile, "name" | "dynamic">) => void;
};

function dynamicDraftRevision(name: string, candidates: DynamicAgentCandidate[]): string {
  return JSON.stringify({ name, candidates });
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
  const { toast } = useToast();
  const enabled = useFeature("dynamicAgentRouting");
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const [name, setName] = useState(profile.name);
  const [candidates, setCandidates] = useState<DynamicAgentCandidate[]>(
    profile.dynamic?.candidates ?? [],
  );
  const [dynamicVersion, setDynamicVersion] = useState(profile.dynamic?.version ?? 1);
  const initialCandidates = profile.dynamic?.candidates ?? [];
  const initialRevision = dynamicDraftRevision(profile.name, initialCandidates);
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

  const notifyDraft = (nextName: string, nextCandidates: DynamicAgentCandidate[]) => {
    onDraftChange?.({
      name: nextName,
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
          rules: { on_provider_error: "try_next" },
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

  const save = async () => {
    if (!enabled || !name.trim() || candidates.length === 0 || !profile.dynamic) return;
    setSaving(true);
    try {
      const draftPayload = {
        name: name.trim(),
        dynamic: {
          version: dynamicVersion,
          candidates,
        },
      };
      const payload = {
        name: name.trim(),
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
      const nextCandidates = updated.dynamic?.candidates ?? candidates;
      setCandidates(nextCandidates);
      setDynamicVersion(updated.dynamic?.version ?? dynamicVersion + 1);
      setSavedRevision(dynamicDraftRevision(updated.name, nextCandidates));
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
  const draftRevision = dynamicDraftRevision(name, candidates);
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
                    className="flex min-w-0 items-center gap-2 rounded-md border p-2"
                  >
                    <Badge variant="outline">{index + 1}</Badge>
                    <span className="min-w-0 flex-1 truncate text-sm">
                      {candidateLabel(candidate, concreteProfiles)}
                    </span>
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
