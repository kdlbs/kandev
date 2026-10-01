import { useFeature } from "@/hooks/domains/features/use-feature";

/** The Projects scope editor exists only while coordinator and phases 2, 3 and 3.1 are all on. */
export function useCoordinatorPhase31Effective(): boolean {
  const coordinator = useFeature("coordinator");
  const phase2 = useFeature("coordinatorPhase2");
  const phase3 = useFeature("coordinatorPhase3");
  const phase31 = useFeature("coordinatorPhase31");
  return coordinator && phase2 && phase3 && phase31;
}
