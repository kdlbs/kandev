import { useTranslation } from "react-i18next";
import type { useSheetData } from "./session-task-switcher-sheet-hooks";

export function useTaskReadStatus(data: ReturnType<typeof useSheetData>) {
  const { t } = useTranslation();
  let taskLoadError: string | null = null;
  if (data.workspaceContextError) {
    taskLoadError = data.workspaceContextAccessDenied
      ? t("sidebar:workspaceContextAccessDenied")
      : t("sidebar:workspaceContextRefreshFailed");
  }
  const retryTaskLoad = data.workspaceContextError ? data.retryWorkspaceContext : data.page.retry;
  return { taskLoadError, retryTaskLoad };
}
