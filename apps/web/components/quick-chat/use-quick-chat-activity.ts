import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import {
  selectQuickChatActivity,
  type QuickChatActivityState,
} from "@/lib/state/slices/ui/quick-chat-activity-selectors";

function quickChatLabelKey(activity: QuickChatActivityState) {
  if (activity === "running") return "sidebar:quickChatRunning";
  if (activity === "finished") return "sidebar:quickChatUnseen";
  return "sidebar:quickChat";
}

export function useQuickChatActivity(workspaceId: string | null | undefined): {
  activity: QuickChatActivityState;
  label: string;
} {
  const { t } = useTranslation();
  const activity = useAppStore((state) => selectQuickChatActivity(state, workspaceId));
  const label = t(quickChatLabelKey(activity));

  return { activity, label };
}
