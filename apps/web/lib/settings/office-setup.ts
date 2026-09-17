import { workspaceSettingsHref } from "./workspace-settings-tabs";
export function officeSetupHref(onboardingComplete: boolean, workspaceId: string | null): string {
  if (!onboardingComplete) return "/?home=overview";
  return workspaceId ? workspaceSettingsHref(workspaceId, "agents") : "/settings/workspaces";
}

export function workspaceOfficeHref(
  inOffice: boolean,
  workspaceId: string | null,
  configured: boolean,
): string {
  if (inOffice) {
    const params = new URLSearchParams({ home: "overview" });
    if (workspaceId) params.set("workspaceId", workspaceId);
    return `/?${params.toString()}`;
  }
  if (configured && workspaceId) return `/office?workspaceId=${encodeURIComponent(workspaceId)}`;
  return officeSetupHref(true, workspaceId);
}
