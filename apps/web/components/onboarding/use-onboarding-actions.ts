"use client";

import { type Dispatch, type SetStateAction, useCallback, useRef, useState } from "react";
import type { ProfileFormData } from "@/components/settings/profile-form-fields";
import type { AgentSetting } from "@/components/onboarding/step-agents";
import { permissionsToProfilePatch } from "@/lib/agent-permissions";
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
      await Promise.all(
        Object.values(agentSettings)
          .filter((setting) => setting.dirty)
          .map((setting) =>
            updateAgentProfileAction(setting.profileId, {
              model: setting.formData.model,
              ...permissionsToProfilePatch(setting.formData),
              cli_passthrough: setting.formData.cli_passthrough,
              cli_flags: setting.formData.cli_flags,
              command_prefix: setting.formData.command_prefix,
            }),
          ),
      );
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
  }, [agentSettings, t, toast]);

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
  const updateSetting = (agentName: string, formPatch: Partial<ProfileFormData>) => {
    setAgentSettings((previous) => ({
      ...previous,
      [agentName]: {
        ...previous[agentName],
        formData: { ...previous[agentName].formData, ...formPatch },
        dirty: true,
      },
    }));
  };

  return { handleSkip, handleNext, handleBack, handleGetStarted, updateSetting, isSaving };
}
