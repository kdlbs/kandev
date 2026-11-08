import { useCoordinatorPhase31Effective } from "@/hooks/domains/settings/use-coordinator-phase31-effective";
import { useFeature } from "@/hooks/domains/features/use-feature";

/** Notes exist only while coordinator, phases 2, 3 and 3.1, and the notes flag are all on. */
export function useCoordinatorNotesEffective(): boolean {
  const phase31 = useCoordinatorPhase31Effective();
  const notes = useFeature("coordinatorPhase35");
  return phase31 && notes;
}
