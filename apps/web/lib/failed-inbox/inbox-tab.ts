export type InboxTab = "needs-you" | "failed" | "history";

export const INBOX_TAB_QUERY_PARAM = "tab";

// AC-UI-INBOX-FAILED-001.2: the `tab` query parameter's only recognised
// values are `needs-you`, `failed`, and `history`. An absent, empty,
// repeated, or unrecognised value renders Needs you rather than an error --
// "repeated" means exactly one occurrence is required, even if every
// repeated value is individually valid.
export function resolveInboxTab(searchParams: URLSearchParams): InboxTab {
  const values = searchParams.getAll(INBOX_TAB_QUERY_PARAM);
  if (values.length !== 1) return "needs-you";
  if (values[0] === "failed") return "failed";
  if (values[0] === "history") return "history";
  return "needs-you";
}

// Selecting a tab REPLACES the current history entry (AC-UI-INBOX-FAILED-001.2),
// carrying the selected tab alongside any other existing query parameters.
export function buildInboxTabHref(
  pathname: string,
  tab: InboxTab,
  currentSearchParams: URLSearchParams = new URLSearchParams(),
): string {
  const params = new URLSearchParams(currentSearchParams);
  params.set(INBOX_TAB_QUERY_PARAM, tab);
  return `${pathname.split("?")[0]}?${params.toString()}`;
}
