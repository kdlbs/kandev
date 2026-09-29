import { useFeature } from "@/hooks/domains/features/use-feature";

/** The autonomy surface exists only while coordinator, phase 2 and phase 3 are all on. */
export function useCoordinatorPhase3Effective(): boolean {
  const coordinator = useFeature("coordinator");
  const phase2 = useFeature("coordinatorPhase2");
  const phase3 = useFeature("coordinatorPhase3");
  return coordinator && phase2 && phase3;
}
