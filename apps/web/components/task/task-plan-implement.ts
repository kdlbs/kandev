import type { TaskPlan } from "@/lib/types/http";

type PlanToolbarImplementArgs = {
  draftContent: string;
  plan: TaskPlan | null;
};

export type PlanToolbarImplementState = {
  visible: boolean;
  disabled: boolean;
  disabledReason?: string;
};

export function getPlanToolbarImplementState({
  draftContent,
  plan,
}: PlanToolbarImplementArgs): PlanToolbarImplementState {
  if (draftContent.trim() === "") {
    return { visible: false, disabled: false };
  }
  if (plan?.implementation_started_at) {
    return {
      visible: true,
      disabled: true,
      disabledReason: "This plan has already been sent for implementation.",
    };
  }
  return { visible: true, disabled: false };
}
