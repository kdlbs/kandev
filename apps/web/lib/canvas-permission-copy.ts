import type { CanvasPermissionReview, CanvasReleaseStatus } from "./api/domains/canvas-api";

export type CanvasPermissionTranslator = (key: string, options?: Record<string, unknown>) => string;

export type CanvasPermissionRow = {
  id: string;
  label: string;
  detail?: string;
  isNew: boolean;
  isUnsupported: boolean;
};

export type CanvasPermissionGroup = {
  id: string;
  label: string;
  rows: CanvasPermissionRow[];
};

const API_READ_KIND = "api_read";
const API_WRITE_KIND = "api_write";
const EVENTS_KIND = "events";
const READS_GROUP = "reads";
const WRITES_GROUP = "writes";
const EVENTS_GROUP = "events";
const SHARED_STATE_GROUP = "shared-state";
const EXTERNAL_ORIGINS_GROUP = "external-origins";
const PERMISSION_GROUP_KIND: Record<string, string> = {
  [READS_GROUP]: API_READ_KIND,
  [WRITES_GROUP]: API_WRITE_KIND,
  [EVENTS_GROUP]: EVENTS_KIND,
};

function permissionKey(group: string, value: string): string {
  if (group === SHARED_STATE_GROUP) return "state";
  if (group === EXTERNAL_ORIGINS_GROUP) return `network:${value}`;
  return `${PERMISSION_GROUP_KIND[group] ?? EVENTS_KIND}:${value}`;
}

function permissionAliases(group: string, value: string): string[] {
  const key = permissionKey(group, value);
  if (group === READS_GROUP && value.endsWith(".read"))
    return [key, `api_read:${value.slice(0, -5)}`];
  if (group === WRITES_GROUP && value.endsWith(".write")) {
    return [key, `api_write:${value.slice(0, -6)}`];
  }
  return [key];
}

function isExactHTTPSOrigin(value: string): boolean {
  try {
    const url = new URL(value);
    return (
      url.protocol === "https:" &&
      url.pathname === "/" &&
      !url.search &&
      !url.hash &&
      !url.username &&
      !url.password
    );
  } catch {
    return false;
  }
}

function rowCopy(
  group: string,
  value: string,
  t: CanvasPermissionTranslator,
): Pick<CanvasPermissionRow, "label" | "detail" | "isUnsupported"> {
  if (group === EXTERNAL_ORIGINS_GROUP) {
    return {
      label: t("canvases:permissionExternalOrigin"),
      detail: value,
      isUnsupported: !isExactHTTPSOrigin(value),
    };
  }
  if (group === SHARED_STATE_GROUP) {
    return {
      label: t("canvases:permissionSharedState"),
      isUnsupported: false,
    };
  }

  const known: Record<string, string> = {
    // These are wire permission identifiers, not user-facing copy.
    [`${READS_GROUP}:tasks`]: "canvases:permissionReadTasks",
    [`${READS_GROUP}:tasks.read`]: "canvases:permissionReadTasks",
    [`${READS_GROUP}:workflows`]: "canvases:permissionReadWorkflows",
    [`${READS_GROUP}:workflows.read`]: "canvases:permissionReadWorkflows",
    [`${WRITES_GROUP}:tasks`]: "canvases:permissionWriteTasks",
    [`${WRITES_GROUP}:tasks.write`]: "canvases:permissionWriteTasks",
    [`${WRITES_GROUP}:messages`]: "canvases:permissionWriteMessages",
    [`${WRITES_GROUP}:messages.write`]: "canvases:permissionWriteMessages",
    [`${EVENTS_GROUP}:task.updated`]: "canvases:permissionEventTaskUpdated",
    [`${EVENTS_GROUP}:workflow.updated`]: "canvases:permissionEventWorkflowUpdated",
  };
  const copyKey = known[`${group}:${value}`];
  if (copyKey) return { label: t(copyKey), isUnsupported: false };
  return {
    label: t("canvases:unsupportedPermission", { value }),
    isUnsupported: true,
  };
}

function buildRows(
  group: string,
  values: string[],
  missing: Set<string>,
  t: CanvasPermissionTranslator,
): CanvasPermissionRow[] {
  return values.map((value, index) => {
    const copy = rowCopy(group, value, t);
    return {
      id: `${group}-${value}-${index}`,
      ...copy,
      isNew: permissionAliases(group, value).some((key) => missing.has(key)),
    };
  });
}

export function buildCanvasPermissionGroups(
  permissions: CanvasPermissionReview | undefined,
  missingPermissions: string[] | undefined,
  t: CanvasPermissionTranslator,
): CanvasPermissionGroup[] {
  if (!permissions) return [];
  const missing = new Set(missingPermissions ?? []);
  const groups: CanvasPermissionGroup[] = [
    {
      id: "reads",
      label: t("canvases:permissionReads"),
      rows: buildRows("reads", permissions.reads ?? [], missing, t),
    },
    {
      id: "writes",
      label: t("canvases:permissionWrites"),
      rows: buildRows("writes", permissions.writes ?? [], missing, t),
    },
    {
      id: "events",
      label: t("canvases:permissionEvents"),
      rows: buildRows("events", permissions.events ?? [], missing, t),
    },
    {
      id: EXTERNAL_ORIGINS_GROUP,
      label: t("canvases:permissionExternalOrigins"),
      rows: buildRows(EXTERNAL_ORIGINS_GROUP, permissions.external_origins ?? [], missing, t),
    },
  ];
  if (permissions.shared_state) {
    groups.push({
      id: SHARED_STATE_GROUP,
      label: t("canvases:sharedState"),
      rows: buildRows(SHARED_STATE_GROUP, ["state"], missing, t),
    });
  }
  return groups.filter((group) => group.rows.length > 0);
}

export function canvasReleaseStatusLabel(
  status: CanvasReleaseStatus | string,
  releaseId: string,
  activeReleaseId: string | undefined,
  t: CanvasPermissionTranslator,
): string {
  if (status === "valid") {
    return releaseId === activeReleaseId
      ? t("canvases:statusActive")
      : t("canvases:statusPrevious");
  }
  const labels: Record<string, string> = {
    pending_permission: t("canvases:statusPending"),
    invalid: t("canvases:invalidRelease"),
    unavailable: t("canvases:unavailable"),
  };
  return labels[status] ?? t("canvases:unsupportedReleaseStatus", { status });
}

export function formatCanvasReleaseDate(
  createdAt: string | undefined,
  t: CanvasPermissionTranslator,
): string {
  if (!createdAt) return t("canvases:dateUnavailable");
  const date = new Date(createdAt);
  if (Number.isNaN(date.getTime())) return t("canvases:dateUnavailable");
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
    date,
  );
}

export function canvasSourceLabel(
  value: string | undefined,
  t: CanvasPermissionTranslator,
): string {
  return value?.trim() || t("canvases:sourceUnavailable");
}

export function canvasSourceActorLabel(
  value: string | undefined,
  t: CanvasPermissionTranslator,
): string {
  const labels: Record<string, string> = {
    task_agent: t("canvases:sourceActorTaskAgent"),
    user: t("canvases:sourceActorUser"),
    system: t("canvases:sourceActorSystem"),
  };
  return labels[value ?? ""] ?? t("canvases:sourceActorUnknown");
}
