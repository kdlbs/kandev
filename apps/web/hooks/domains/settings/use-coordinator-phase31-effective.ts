import { useFeature } from "@/hooks/domains/features/use-feature";

/** The 3.1 surfaces exist only while coordinator, phases 2 and 3, and phase 3.1 are all on. */
export function useCoordinatorPhase31Effective(): boolean {
  const coordinator = useFeature("coordinator");
  const phase2 = useFeature("coordinatorPhase2");
  const phase3 = useFeature("coordinatorPhase3");
  const phase31 = useFeature("coordinatorPhase31");
  return coordinator && phase2 && phase3 && phase31;
}
