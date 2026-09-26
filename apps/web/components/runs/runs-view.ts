/**
 * The automations destination. Named for the object it lists, not for the runs
 * that object produces: the sidebar section, the settings page, and this all say
 * "Automations", and a path that said `runs` was the last piece still describing
 * the old feed-first shape.
 */
export const AUTOMATIONS_HREF = "/automations";

/**
 * The path this page answered to before the rename. Kept resolving so links
 * already shared or bookmarked keep working.
 */
export const LEGACY_RUNS_PREFIX = "/runs";

/**
 * The flat cross-automation feed, demoted from front door to lens.
 *
 * It lives behind a query parameter on `/runs` rather than its own path
 * segment so that `/runs/<something>` means one thing only — an automation id.
 * A sibling segment like `/runs/all` would be a reserved word competing with
 * the id space, which is exactly the kind of collision that only shows up once
 * something is named badly.
 */
export const RUNS_FEED_VIEW = "feed";

export const RUNS_FEED_HREF = `${AUTOMATIONS_HREF}?view=${RUNS_FEED_VIEW}`;

export type AutomationDetailTab = "activity" | "configure";

/**
 * Activity is the landing view, always. This surface exists because reading was
 * previously buried under editing, so anything short of an explicit request for
 * the editor resolves to Activity — including a malformed or stale tab value.
 */
export function parseDetailTab(value: string | undefined): AutomationDetailTab {
  return value === "configure" ? "configure" : "activity";
}

/** Both tabs name the automation, so either is bookmarkable on its own. */
export function detailTabHref(automationId: string, tab: AutomationDetailTab): string {
  const base = `${AUTOMATIONS_HREF}/${automationId}`;
  return tab === "activity" ? base : `${base}?tab=${tab}`;
}

/**
 * A specific run of an automation. The rail switches which run the pane shows,
 * and that selection belongs in the URL: "the run that failed overnight" is a
 * thing people link each other to, and a selection held only in component state
 * dies on refresh.
 */
export function runHref(automationId: string, runId: string): string {
  return `${AUTOMATIONS_HREF}/${automationId}?run=${encodeURIComponent(runId)}`;
}
