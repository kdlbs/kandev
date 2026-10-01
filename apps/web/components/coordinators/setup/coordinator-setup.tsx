"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkspaceBoards } from "@/hooks/domains/coordinator/use-workspace-boards";
import { useWorkspaceProjects } from "@/hooks/domains/coordinator/use-workspace-projects";
import { useCoordinatorPhase31Effective } from "@/hooks/domains/settings/use-coordinator-phase31-effective";
import { setupCoordinator } from "@/lib/api/domains/coordinator-api";
import {
  resolveDefaultAgentProfileId,
  resolveDefaultExecutorProfileId,
} from "@/lib/coordinators/coordinator-form";
import {
  activeSetupSteps,
  buildSetupRequest,
  gotRefusal,
  initialSetupState,
  isSetupValid,
  setupServerError,
  skipStep,
  type SetupServerError,
  type SetupState,
  type SetupStepId,
} from "@/lib/coordinators/setup";
import type { WorkspaceState } from "@/lib/state/slices";
import { SetupStepList } from "./setup-nav";
import { SetupReview } from "./setup-review";
import {
  ContextStep,
  GoalStep,
  IdentityStep,
  MayDoStep,
  ProjectsStep,
  WatchesStep,
} from "./setup-step-bodies";
import { useSetupForm } from "./use-setup-form";

type Workspace = WorkspaceState["items"][number];
type Banner = "not-created" | "unconfirmed" | null;

type Props = { workspaceId: string };

type FinishArgs = {
  workspaceId: string;
  state: SetupState;
  onRefused: (refusal: SetupServerError & { step: SetupStepId }) => void;
};

function useSetupFinish({ workspaceId, state, onRefused }: FinishArgs) {
  const router = useRouter();
  const addCoordinator = useAppStore((s) => s.addCoordinator);
  const [busy, setBusy] = useState(false);
  const [banner, setBanner] = useState<Banner>(null);

  const finish = async () => {
    if (busy) return;
    setBusy(true);
    setBanner(null);
    try {
      const created = await setupCoordinator(workspaceId, buildSetupRequest(state));
      if (!created?.id) throw new Error("setup response has no coordinator");
      addCoordinator(created);
      router.replace(`/settings/workspaces/${workspaceId}/coordinators/${created.id}`);
    } catch (error) {
      const refusal = setupServerError(error);
      if (refusal?.step) {
        onRefused({ ...refusal, step: refusal.step });
      } else {
        setBanner(gotRefusal(error) ? "not-created" : "unconfirmed");
      }
      setBusy(false);
    }
  };
  return { busy, banner, finish };
}

function SetupBanner({ banner }: { banner: Banner }) {
  const { t } = useTranslation();
  if (!banner) return null;
  return (
    <p role="alert" className="text-sm text-destructive" data-testid={`setup-banner-${banner}`}>
      {t(banner === "not-created" ? "coordinator:setupNotCreated" : "coordinator:setupUnconfirmed")}
    </p>
  );
}

type FooterProps = {
  step: SetupStepId;
  index: number;
  busy: boolean;
  nextEnabled: boolean;
  canFinish: boolean;
  onBack: () => void;
  onSkip: () => void;
  onNext: () => void;
  onFinish: () => void;
};

const BUTTON_CLASS = "min-h-11 cursor-pointer sm:min-h-9";

function SetupFooter(props: FooterProps) {
  const { t } = useTranslation();
  const { step, index, busy } = props;
  const skippable = step === "projects" || step === "goal" || step === "context";
  return (
    <div className="flex flex-wrap items-center gap-2">
      {index > 0 && (
        <Button
          type="button"
          variant="outline"
          className={BUTTON_CLASS}
          data-testid="setup-back"
          disabled={busy}
          onClick={props.onBack}
        >
          {t("coordinator:setupBack")}
        </Button>
      )}
      {skippable && (
        <Button
          type="button"
          variant="ghost"
          className={BUTTON_CLASS}
          data-testid="setup-skip"
          onClick={props.onSkip}
        >
          {t("coordinator:setupSkip")}
        </Button>
      )}
      {step !== "review" ? (
        <Button
          type="button"
          className={BUTTON_CLASS}
          data-testid="setup-next"
          disabled={!props.nextEnabled}
          onClick={props.onNext}
        >
          {t("coordinator:setupNext")}
        </Button>
      ) : (
        <Button
          type="button"
          className={BUTTON_CLASS}
          data-testid="setup-finish"
          disabled={!props.canFinish}
          onClick={props.onFinish}
        >
          {t("coordinator:setupFinish")}
        </Button>
      )}
    </div>
  );
}

export function CoordinatorSetup({ workspaceId }: Props) {
  const { t } = useTranslation();
  useSettingsData(true);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const executors = useAppStore((s) => s.executors.items);
  const workspace = useAppStore(
    (s) => s.workspaces.items.find((item: Workspace) => item.id === workspaceId) ?? null,
  );
  const boards = useWorkspaceBoards(workspaceId, true);
  const projectsOffered = useCoordinatorPhase31Effective();
  const projects = useWorkspaceProjects(workspaceId, projectsOffered);
  const steps = activeSetupSteps(projectsOffered);
  const form = useSetupForm(
    initialSetupState({
      agentProfileId: resolveDefaultAgentProfileId(
        workspace?.default_agent_profile_id,
        agentProfiles,
      ),
      executorProfileId: resolveDefaultExecutorProfileId(workspace?.default_executor_id, executors),
    }),
  );
  const { state } = form;

  const [step, setStep] = useState<SetupStepId>("identity");
  const [toReview, setToReview] = useState(false);
  const [left, setLeft] = useState<ReadonlySet<SetupStepId>>(new Set());
  const { busy, banner, finish } = useSetupFinish({
    workspaceId,
    state,
    onRefused: (refusal) => {
      form.reject(refusal);
      setToReview(false);
      setStep(refusal.step);
    },
  });

  const index = steps.indexOf(step);
  const marked = new Set(steps.filter((s) => left.has(s) && form.valid(s)));

  const leave = (from: SetupStepId) => {
    setLeft((prev) => new Set([...prev, from]));
    setStep(toReview ? "review" : steps[index + 1]);
  };
  const skip = () => {
    form.skip(step, skipStep(step, state));
    leave(step);
  };
  const back = () => {
    setToReview(false);
    setStep(steps[index - 1]);
  };
  const change = (target: SetupStepId) => {
    setToReview(true);
    setStep(target);
  };

  const canFinish = isSetupValid(state) && steps.every((s) => form.valid(s)) && !busy;
  const messages = form.messages(step);
  const bodyProps = { state, messages, edit: form.edit, leave: form.leave };

  return (
    <div className="max-w-3xl space-y-6" data-testid="coordinator-setup">
      <h1 className="text-xl font-semibold">{t("coordinator:addCoordinator")}</h1>
      <SetupStepList steps={steps} current={step} marked={marked} />
      <div data-testid={`setup-body-${step}`}>
        {step === "identity" && (
          <IdentityStep {...bodyProps} agentProfiles={agentProfiles} executors={executors} />
        )}
        {step === "watches" && <WatchesStep {...bodyProps} boards={boards} />}
        {step === "projects" && <ProjectsStep {...bodyProps} projects={projects} />}
        {step === "goal" && <GoalStep {...bodyProps} />}
        {step === "context" && (
          <ContextStep {...bodyProps} agentProfiles={agentProfiles} executors={executors} />
        )}
        {step === "may-do" && <MayDoStep {...bodyProps} />}
        {step === "review" && (
          <SetupReview
            state={state}
            agentProfiles={agentProfiles}
            executors={executors}
            boards={boards.boards}
            projectChoices={projectsOffered ? [...projects.sets, ...projects.loose] : undefined}
            onChange={change}
          />
        )}
      </div>
      <SetupBanner banner={banner} />
      <SetupFooter
        step={step}
        index={index}
        busy={busy}
        nextEnabled={form.valid(step)}
        canFinish={canFinish}
        onBack={back}
        onSkip={skip}
        onNext={() => leave(step)}
        onFinish={() => void finish()}
      />
    </div>
  );
}
