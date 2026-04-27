"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { completeOnboarding } from "@/lib/api/domains/orchestrate-api";
import { StepWorkspace } from "./step-workspace";
import { StepAgent } from "./step-agent";
import { StepTask } from "./step-task";
import { StepReview } from "./step-review";
import { WizardFooter } from "./wizard-footer";

type AgentProfile = {
  id: string;
  label: string;
  agentName: string;
};

type WizardData = {
  workspaceName: string;
  taskPrefix: string;
  agentName: string;
  agentProfileId: string;
  executorPreference: string;
  taskTitle: string;
  taskDescription: string;
};

// Initial form values -- these are UI-only placeholder defaults for the wizard fields.
// The backend validates and may override these during onboarding completion.
const INITIAL_DATA: WizardData = {
  workspaceName: "default",
  taskPrefix: "KAN",
  agentName: "CEO",
  agentProfileId: "",
  executorPreference: "local_pc",
  taskTitle: "",
  taskDescription: "",
};

const STEP_COUNT = 4;

function computeCanAdvance(step: number, data: WizardData): boolean {
  if (step === 0) return data.workspaceName.trim() !== "";
  if (step === 1) return data.agentName.trim() !== "";
  return true;
}

function dotColor(index: number, current: number): string {
  if (index === current) return "bg-primary";
  if (index < current) return "bg-primary/50";
  return "bg-muted";
}

type SetupWizardProps = {
  agentProfiles: AgentProfile[];
};

export function SetupWizard({ agentProfiles }: SetupWizardProps) {
  const router = useRouter();
  const [step, setStep] = useState(0);
  const [data, setData] = useState<WizardData>(INITIAL_DATA);
  const [submitting, setSubmitting] = useState(false);

  const patch = useCallback(
    (updates: Partial<WizardData>) => setData((prev) => ({ ...prev, ...updates })),
    [],
  );

  const canAdvance = computeCanAdvance(step, data);

  const handleSubmit = useCallback(async () => {
    setSubmitting(true);
    try {
      await completeOnboarding({
        workspaceName: data.workspaceName.trim() || "default",
        taskPrefix: data.taskPrefix.trim() || "KAN",
        agentName: data.agentName.trim() || "CEO",
        agentProfileId: data.agentProfileId,
        executorPreference: data.executorPreference || "local_pc",
        taskTitle: data.taskTitle.trim() || undefined,
        taskDescription: data.taskDescription.trim() || undefined,
      });
      toast.success("Workspace created successfully");
      router.push("/orchestrate");
      router.refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to complete setup");
    } finally {
      setSubmitting(false);
    }
  }, [data, router]);

  const profileLabel =
    agentProfiles.find((p) => p.id === data.agentProfileId)?.label || "";

  return (
    <div className="fixed inset-0 z-50 bg-background flex items-center justify-center">
      <div className="w-full max-w-2xl mx-auto px-6">
        <StepIndicator current={step} total={STEP_COUNT} />
        <div className="mt-8">
          {step === 0 && (
            <StepWorkspace
              workspaceName={data.workspaceName}
              taskPrefix={data.taskPrefix}
              onChange={patch}
            />
          )}
          {step === 1 && (
            <StepAgent
              agentName={data.agentName}
              agentProfileId={data.agentProfileId}
              executorPreference={data.executorPreference}
              agentProfiles={agentProfiles}
              onChange={patch}
            />
          )}
          {step === 2 && (
            <StepTask
              taskTitle={data.taskTitle}
              taskDescription={data.taskDescription}
              onChange={patch}
            />
          )}
          {step === 3 && (
            <StepReview
              workspaceName={data.workspaceName}
              taskPrefix={data.taskPrefix}
              agentName={data.agentName}
              agentProfileLabel={profileLabel}
              executorPreference={data.executorPreference}
              taskTitle={data.taskTitle}
            />
          )}
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

