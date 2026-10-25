export const SIDEBAR_TASK_ROW_DETAIL_KEYS = [
  "relative_time",
  "repository",
  "pull_request_number",
] as const;

export type SidebarTaskRowDetail = (typeof SIDEBAR_TASK_ROW_DETAIL_KEYS)[number];

export const SIDEBAR_TASK_ROW_TRAILING_KEYS = [
  "git_changes",
  "relative_time",
  "change_request_status",
  "none",
] as const;

export type SidebarTaskRowTrailing = (typeof SIDEBAR_TASK_ROW_TRAILING_KEYS)[number];

export type SidebarTaskRowPresentation = {
  detailsEnabled: boolean;
  detailOrder: SidebarTaskRowDetail[];
  visibleDetails: SidebarTaskRowDetail[];
  trailing: SidebarTaskRowTrailing;
};

export const DEFAULT_SIDEBAR_TASK_ROW_PRESENTATION: SidebarTaskRowPresentation = {
  detailsEnabled: true,
  detailOrder: [...SIDEBAR_TASK_ROW_DETAIL_KEYS],
  visibleDetails: [...SIDEBAR_TASK_ROW_DETAIL_KEYS],
  trailing: "git_changes",
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isDetail(value: unknown): value is SidebarTaskRowDetail {
  return (
    typeof value === "string" && (SIDEBAR_TASK_ROW_DETAIL_KEYS as readonly string[]).includes(value)
  );
}

function isTrailing(value: unknown): value is SidebarTaskRowTrailing {
  return (
    typeof value === "string" &&
    (SIDEBAR_TASK_ROW_TRAILING_KEYS as readonly string[]).includes(value)
  );
}

function normalizeDetails(value: unknown, fallback: readonly SidebarTaskRowDetail[]) {
  if (!Array.isArray(value)) return [...fallback];
  const seen = new Set<SidebarTaskRowDetail>();
  const normalized = value.filter(isDetail).filter((detail) => {
    if (seen.has(detail)) return false;
    seen.add(detail);
    return true;
  });
  return normalized;
}

/** Normalizes persisted or boot-provided task-row data without mutating it. */
export function normalizeSidebarTaskRowPresentation(value: unknown): SidebarTaskRowPresentation {
  const input = isRecord(value) ? value : {};
  const detailOrder = normalizeDetails(input.detailOrder, SIDEBAR_TASK_ROW_DETAIL_KEYS);
  for (const detail of SIDEBAR_TASK_ROW_DETAIL_KEYS) {
    if (!detailOrder.includes(detail)) detailOrder.push(detail);
  }

  const visibleDetails = Array.isArray(input.visibleDetails)
    ? normalizeDetails(input.visibleDetails, [])
    : [...SIDEBAR_TASK_ROW_DETAIL_KEYS];

  return {
    detailsEnabled: typeof input.detailsEnabled === "boolean" ? input.detailsEnabled : true,
    detailOrder,
    visibleDetails,
    trailing: isTrailing(input.trailing) ? input.trailing : "git_changes",
  };
}

export function cloneSidebarTaskRowPresentation(
  value: SidebarTaskRowPresentation | undefined,
): SidebarTaskRowPresentation {
  const normalized = normalizeSidebarTaskRowPresentation(value);
  return {
    detailsEnabled: normalized.detailsEnabled,
    detailOrder: [...normalized.detailOrder],
    visibleDetails: [...normalized.visibleDetails],
    trailing: normalized.trailing,
  };
}
