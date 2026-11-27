export const INTEGRATED_CONTROL_HREF = "/control";
export const CONTROL_VIEWS = [
  "overview",
  "activity",
  "workspaces",
  "providers",
  "history",
] as const;
export type ControlView = (typeof CONTROL_VIEWS)[number];
export const CONTROL_VIEW_LABEL_KEYS = {
  overview: "sidebar:integratedControl.views.overview",
  activity: "sidebar:integratedControl.views.activity",
  workspaces: "common:workspaces",
  providers: "sidebar:integratedControl.views.providers",
  history: "sidebar:integratedControl.views.history",
} as const satisfies Record<ControlView, string>;

export function isIntegratedControlPath(pathname: string): boolean {
  return pathname === INTEGRATED_CONTROL_HREF || pathname.startsWith(`${INTEGRATED_CONTROL_HREF}/`);
}

export function controlView(pathname: string): ControlView {
  const segment = pathname.slice(INTEGRATED_CONTROL_HREF.length + 1);
  return CONTROL_VIEWS.find((view) => view === segment) ?? "overview";
}

export function controlHref(
  view: ControlView,
  workspaceId?: string,
  coordinatorId?: string,
): string {
  const query = new URLSearchParams();
  if (workspaceId) query.set("workspace", workspaceId);
  if (coordinatorId) query.set("coordinator", coordinatorId);
  const suffix = query.size > 0 ? `?${query}` : "";
  return `${INTEGRATED_CONTROL_HREF}/${view}${suffix}`;
}
