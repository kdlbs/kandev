import { useEffect, useState } from "react";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
export const ONBOARDING_CHANGED = "kandev:onboarding-completed";
export function useKanbanOnboardingComplete() {
  const read = () => getLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, false);
  const [completed, setCompleted] = useState(read);
  useEffect(() => {
    const refresh = () => setCompleted(read());
    window.addEventListener(ONBOARDING_CHANGED, refresh);
    window.addEventListener("storage", refresh);
    return () => {
      window.removeEventListener(ONBOARDING_CHANGED, refresh);
      window.removeEventListener("storage", refresh);
    };
  }, []);
  return completed;
}
