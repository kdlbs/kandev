"use client";

import { useState, useCallback } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { toast } from "sonner";
import {
  completeOnboarding,
  importFromFS,
  type OnboardingFSWorkspace,
} from "@/lib/api/domains/office-api";
import { StepImport } from "./step-import";
import { StepWorkspace, derivePrefix } from "./step-workspace";
import { StepTierProfiles } from "./step-tier-profiles";
import { StepAgent } from "./step-agent";
import { StepTask } from "./step-task";
import { StepReview } from "./step-review";
import { WizardFooter } from "./wizard-footer";
import { CloseButton } from "./close-button";
import {
  DEFAULT_ONBOARDING_TASK_DESCRIPTION,
  DEFAULT_ONBOARDING_TASK_TITLE,
} from "./setup-task-defaults";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Tier } from "@/lib/state/slices/office/types";

export { DEFAULT_ONBOARDING_TASK_DESCRIPTION, DEFAULT_ONBOARDING_TASK_TITLE };

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
  tierProfileIds: Partial<Record<Tier, string>>;
  executorPreference: string;
  defaultTier?: Tier;
  taskTitle: string;
  taskDescription: string;
};

export function getInitialData(
  suggestedWorkspaceName: string,
  defaultAgentProfileId?: string,
): WizardData {
  const initialTierProfileIds = defaultAgentProfileId
    ? {
        frontier: defaultAgentProfileId,
        balanced: defaultAgentProfileId,
        economy: defaultAgentProfileId,
      }
    : {};
  return {
    workspaceName: suggestedWorkspaceName,
    taskPrefix: derivePrefix(suggestedWorkspaceName),
    agentName: "CEO",
    agentProfileId: defaultAgentProfileId ?? "",
    tierProfileIds: initialTierProfileIds,
    executorPreference: "local_pc",
    defaultTier: "frontier",
    taskTitle: DEFAULT_ONBOARDING_TASK_TITLE,
    taskDescription: DEFAULT_ONBOARDING_TASK_DESCRIPTION,
  };
}

const STEP_COUNT = 5;

function computeCanAdvance(step: number, data: WizardData): boolean {
  if (step === 0) return data.workspaceName.trim() !== "";
  if (step === 1)
    return (
      Boolean(data.tierProfileIds.frontier) &&
      Boolean(data.tierProfileIds.balanced) &&
      Boolean(data.tierProfileIds.economy)
    );
  if (step === 2) return data.agentName.trim() !== "" && data.agentProfileId !== "";
  return true;
}

function dotColor(index: number, current: number): string {
  if (index === current) return "bg-primary";
  if (index < current) return "bg-primary/50";
  return "bg-muted";
}

export async function submitOnboarding(data: WizardData) {
  const result = await completeOnboarding({
    workspaceName: data.workspaceName.trim() || "default",
    taskPrefix: data.taskPrefix.trim() || "KAN",
    agentName: data.agentName.trim() || "CEO",
    agentProfileId: data.agentProfileId,
    tier_profiles: {
      frontier: data.tierProfileIds.frontier,
      balanced: data.tierProfileIds.balanced,
      economy: data.tierProfileIds.economy,
    },
    executorPreference: data.executorPreference || "local_pc",
    taskTitle: data.taskTitle.trim() || undefined,
    taskDescription: data.taskDescription.trim() || undefined,
    default_tier: data.defaultTier,
  });
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
      <StepTierProfiles
        tierProfileIds={data.tierProfileIds}
        agentProfiles={agentProfiles}
        onChange={patch}
        onAgentProfilesChange={onAgentProfilesChange}
      />
    );
  if (step === 2)
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
  if (step === 3)
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
  const skipInitialTask = useCallback(() => {
    patch({ taskTitle: "", taskDescription: "" });
    setStep((s) => s + 1);
  }, [patch]);
  const canAdvance = computeCanAdvance(step, data);

  const handleSubmit = useCallback(async () => {
    setSubmitting(true);
    try {
      const result = await submitOnboarding(data);
      toast.success("Workspace created successfully");
      document.cookie = `office-active-workspace=${result.workspaceId}; path=/; max-age=86400; samesite=strict; secure`;
      router.push(`/office?workspaceId=${result.workspaceId}`);
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

  const closeHref = "/";

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
          onSkip={skipInitialTask}
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
