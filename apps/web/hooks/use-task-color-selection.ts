import { useAppStore } from "@/components/state-provider";
import { useSetTaskColors } from "./use-task-color";

export function useTaskColorSelection(taskIds: string[]) {
  const colors = useAppStore((state) => state.userSettings.sidebarTaskColors);
  const mutation = useSetTaskColors();
  const ids = [...new Set(taskIds.filter(Boolean))];
  const first = colors[ids[0]] ?? null;
  const commonColor = ids.every((id) => (colors[id] ?? null) === first) ? first : null;
  const hasColor = ids.some((id) => colors[id] != null);
  return { ...mutation, ids, commonColor, hasColor };
}
