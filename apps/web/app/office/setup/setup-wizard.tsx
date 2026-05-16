"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import {
  completeOnboarding,
  importFromFS,
  type OnboardingFSWorkspace,
} from "@/lib/api/domains/office-api";
import { StepImport } from "./step-import";
import { StepWorkspace, derivePrefix } from "./step-workspace";
import { StepAgent } from "./step-agent";
import { StepTask } from "./step-task";
import { StepReview } from "./step-review";
import { WizardFooter } from "./wizard-footer";
import { CloseButton } from "./close-button";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { Tier } from "@/lib/state/slices/office/types";

type SetupWizardProps = {
  agentProfiles: AgentProfileOption[];
  fsWorkspaces: OnboardingFSWorkspace[];
  mode?: string;
  /**
   * Sensible default agent profile pre-selected for the coordinator. Resolved
   * server-side from the user's `default_utility_agent_id` falling back to
   * the first installed CLI profile. Empty when no profiles exist yet —
   * the wizard then surfaces a "Set up CLI in Settings" link.
   */
  defaultAgentProfileId?: string;
  /**
   * Pre-filled workspace name. "Default" on first onboarding; for additional
   * workspaces ("Add workspace" flow) the page resolves to the first unused
   * "Default N" so we never collide with an existing office workspace.
   */
  suggestedWorkspaceName: string;
};

type WizardData = {
  workspaceName: string;
  taskPrefix: string;
  agentName: string;
  agentProfileId: string;
  executorPreference: string;
  defaultTier?: Tier;
  taskTitle: string;
  taskDescription: string;
};

function getInitialData(
  suggestedWorkspaceName: string,
  defaultAgentProfileId?: string,
): WizardData {
  return {
    workspaceName: suggestedWorkspaceName,
    taskPrefix: derivePrefix(suggestedWorkspaceName),
    agentName: "CEO",
    agentProfileId: defaultAgentProfileId ?? "",
    executorPreference: "local_pc",
    taskTitle: "",
    taskDescription: "",
  };
}

const STEP_COUNT = 4;

function computeCanAdvance(step: number, data: WizardData): boolean {
  if (step === 0) return data.workspaceName.trim() !== "";
  if (step === 1) return data.agentName.trim() !== "" && data.agentProfileId !== "";
  return true;
}

function dotColor(index: number, current: number): string {
  if (index === current) return "bg-primary";
  if (index < current) return "bg-primary/50";
  return "bg-muted";
}

async function submitOnboarding(data: WizardData) {
  const result = await completeOnboarding({
    workspaceName: data.workspaceName.trim() || "default",
    taskPrefix: data.taskPrefix.trim() || "KAN",
    agentName: data.agentName.trim() || "CEO",
    agentProfileId: data.agentProfileId,
    executorPreference: data.executorPreference || "local_pc",
    taskTitle: data.taskTitle.trim() || undefined,
    taskDescription: data.taskDescription.trim() || undefined,
    default_tier: data.defaultTier,
  });
  await updateUserSettings({ workspace_id: result.workspaceId });
  return result;
}

function WizardStepContent({
  step,
  data,
  agentProfiles,
  patch,
  onAgentProfilesChange,
}: {
  step: number;
  data: WizardData;
  agentProfiles: AgentProfileOption[];
  patch: (updates: Partial<WizardData>) => void;
  onAgentProfilesChange: (profiles: AgentProfileOption[]) => void;
}) {
  if (step === 0)
    return (
      <StepWorkspace
        workspaceName={data.workspaceName}
        taskPrefix={data.taskPrefix}
        onChange={patch}
      />
    );
  if (step === 1)
    return (
      <StepAgent
        agentName={data.agentName}
        agentProfileId={data.agentProfileId}
        executorPreference={data.executorPreference}
        defaultTier={data.defaultTier}
        agentProfiles={agentProfiles}
        onChange={patch}
        onAgentProfilesChange={onAgentProfilesChange}
      />
    );
  if (step === 2)
    return (
      <StepTask
        agentName={data.agentName}
        taskTitle={data.taskTitle}
        taskDescription={data.taskDescription}
        onChange={patch}
      />
    );
  return (
    <StepReview
      workspaceName={data.workspaceName}
      taskPrefix={data.taskPrefix}
      agentName={data.agentName}
      agentProfileLabel={agentProfiles.find((p) => p.id === data.agentProfileId)?.label || ""}
      executorPreference={data.executorPreference}
      taskTitle={data.taskTitle}
    />
  );
}

export function SetupWizard({
  agentProfiles,
  fsWorkspaces,
  mode,
  defaultAgentProfileId,
  suggestedWorkspaceName,
}: SetupWizardProps) {
  const router = useRouter();
  // mode === "new" means the user explicitly asked for a fresh workspace
  // (e.g. clicked "Add workspace" on the dashboard) — skip the FS import
  // prompt even when on-disk configs exist.
  const [showWizard, setShowWizard] = useState(mode === "new" || fsWorkspaces.length === 0);
  const [step, setStep] = useState(0);
  const [profileOptions, setProfileOptions] = useState(agentProfiles);
  const [data, setData] = useState<WizardData>(() =>
    getInitialData(suggestedWorkspaceName, defaultAgentProfileId),
  );
  const [submitting, setSubmitting] = useState(false);
  const patch = useCallback(
    (updates: Partial<WizardData>) => setData((prev) => ({ ...prev, ...updates })),
    [],
  );
  const canAdvance = computeCanAdvance(step, data);

  const handleSubmit = useCallback(async () => {
    setSubmitting(true);
    try {
      await submitOnboarding(data);
      toast.success("Workspace created successfully");
      router.push("/office");
      router.refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to complete setup");
    } finally {
      setSubmitting(false);
    }
  }, [data, router]);

  const handleImportFS = useCallback(async () => {
    setSubmitting(true);
    try {
      const result = await importFromFS();
      toast.success(`Imported ${result.importedCount} config entries`);
      router.push("/office");
      router.refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to import settings");
    } finally {
      setSubmitting(false);
    }
  }, [router]);

  const closeHref = mode === "new" ? "/office" : "/";

  if (!showWizard) {
    return (
      <StepImport
        fsWorkspaces={fsWorkspaces}
        submitting={submitting}
        onImport={handleImportFS}
        onSkip={() => setShowWizard(true)}
        closeHref={closeHref}
      />
    );
  }
  return (
    <div className="fixed inset-0 z-50 bg-background flex items-center justify-center">
      <div className="relative w-full max-w-2xl mx-auto px-6">
        <CloseButton href={closeHref} />
        <StepIndicator current={step} total={STEP_COUNT} />
        <div className="mt-8">
          <WizardStepContent
            step={step}
            data={data}
            agentProfiles={profileOptions}
            patch={patch}
            onAgentProfilesChange={setProfileOptions}
          />
        </div>
        <WizardFooter
          step={step}
          canAdvance={canAdvance}
          submitting={submitting}
          onBack={() => setStep((s) => s - 1)}
          onNext={() => setStep((s) => s + 1)}
          onSkip={() => setStep((s) => s + 1)}
          onSubmit={handleSubmit}
        />
      </div>
    </div>
  );
}

function StepIndicator({ current, total }: { current: number; total: number }) {
  return (
    <div className="flex items-center justify-center gap-2">
      {Array.from({ length: total }, (_, i) => (
        <div key={i} className={`h-2 w-2 rounded-full transition-colors ${dotColor(i, current)}`} />
      ))}
    </div>
  );
}
