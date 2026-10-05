"use client";

import { type Dispatch, type SetStateAction, useCallback, useRef, useState } from "react";
import type { AgentSetting, OnboardingAgentDraft } from "@/components/onboarding/agent-settings";
import { updateAgentProfileAction } from "@/app/actions/agents";
import { isHandledApiError } from "@/lib/api/client";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";

export const TOTAL_ONBOARDING_STEPS = 4;

type OnboardingActionsProps = {
  step: number;
  setStep: Dispatch<SetStateAction<number>>;
  onComplete: () => void;
  agentSettings: Record<string, AgentSetting>;
  setAgentSettings: Dispatch<SetStateAction<Record<string, AgentSetting>>>;
};

function updateSavedSetting(
  settings: Record<string, AgentSetting>,
  profileId: string,
  patch: Partial<OnboardingAgentDraft>,
): Record<string, AgentSetting> {
  const entry = Object.entries(settings).find(([, setting]) => setting.profileId === profileId);
  if (!entry) return settings;
  const [agentName, current] = entry;
  const baseline = { ...current.baseline, ...patch };
  return {
    ...settings,
    [agentName]: {
      ...current,
      baseline,
      dirty:
        current.draft.model !== baseline.model ||
        current.draft.cli_passthrough !== baseline.cli_passthrough,
    },
  };
}

export function useOnboardingActions({
  step,
  setStep,
  onComplete,
  agentSettings,
  setAgentSettings,
}: OnboardingActionsProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const saveInProgressRef = useRef(false);
  const [isSaving, setIsSaving] = useState(false);

  const saveAgentSettings = useCallback(async (): Promise<boolean> => {
    try {
      const dirtySettings = Object.values(agentSettings).filter((setting) => setting.dirty);
      const saves = await Promise.allSettled(
        dirtySettings.map(async (setting) => {
          const patch: { model?: string; cli_passthrough?: boolean } = {};
          if (setting.draft.model !== setting.baseline.model) {
            patch.model = setting.draft.model;
          }
          if (setting.draft.cli_passthrough !== setting.baseline.cli_passthrough) {
            patch.cli_passthrough = setting.draft.cli_passthrough;
          }
          if (Object.keys(patch).length > 0) {
            await updateAgentProfileAction(setting.profileId, patch);
            setAgentSettings((current) => updateSavedSetting(current, setting.profileId, patch));
          }
        }),
      );
      const failedSave = saves.find((save) => save.status === "rejected");
      if (failedSave) throw failedSave.reason;
      return true;
    } catch (error) {
      if (isHandledApiError(error)) return false;
      toast({
        title: t("common:error"),
        description: t("common:onboardingAgentSettingsSaveError"),
        variant: "error",
      });
      return false;
    }
  }, [agentSettings, setAgentSettings, t, toast]);

  const saveAgentSettingsOnce = async (): Promise<boolean> => {
    if (saveInProgressRef.current) return false;
    saveInProgressRef.current = true;
    setIsSaving(true);
    try {
      return await saveAgentSettings();
    } finally {
      saveInProgressRef.current = false;
      setIsSaving(false);
    }
  };

  const handleSkip = () => {
    if (saveInProgressRef.current) return;
    onComplete();
    setStep(0);
  };
  const handleNext = async () => {
    if (saveInProgressRef.current) return;
    if (step === 0 && !(await saveAgentSettingsOnce())) return;
    if (step < TOTAL_ONBOARDING_STEPS - 1) setStep(step + 1);
  };
  const handleBack = () => {
    if (saveInProgressRef.current) return;
    if (step > 0) setStep(step - 1);
  };
  const handleGetStarted = async () => {
    if (saveInProgressRef.current || !(await saveAgentSettingsOnce())) return;
    onComplete();
    setStep(0);
  };
  const updateSetting = useCallback(
    (agentName: string, patch: Partial<OnboardingAgentDraft>) => {
      setAgentSettings((previous) => {
        const current = previous[agentName];
        if (!current) return previous;
        const nextDraft = { ...current.draft, ...patch };
        const isDirty =
          nextDraft.model !== current.baseline.model ||
          nextDraft.cli_passthrough !== current.baseline.cli_passthrough;
        return {
          ...previous,
          [agentName]: {
            ...current,
            draft: nextDraft,
            dirty: isDirty,
          },
        };
      });
    },
    [setAgentSettings],
  );

  return { handleSkip, handleNext, handleBack, handleGetStarted, updateSetting, isSaving };
}
