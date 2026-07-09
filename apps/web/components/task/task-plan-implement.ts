import type { TaskPlan } from "@/lib/types/http";

type PlanToolbarImplementArgs = {
  draftContent: string;
  plan: TaskPlan | null;
};

export function shouldShowPlanToolbarImplement({
  draftContent,
  plan,
}: PlanToolbarImplementArgs): boolean {
  if (draftContent.trim() === "") return false;
  return !plan?.implementation_started_at;
}
