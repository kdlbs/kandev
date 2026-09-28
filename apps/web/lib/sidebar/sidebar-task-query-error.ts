import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

export function sidebarTaskQueryError(error: unknown, hasPage: boolean, t: TFunction) {
  if (error instanceof ApiError && [401, 403, 404].includes(error.status)) {
    return { message: t("sidebar:workspaceContextAccessDenied"), canRetry: false };
  }
  if (!(error instanceof ApiError) || error.errorCode !== "sidebar_query_invalid") {
    return {
      message: t(hasPage ? "sidebar:queryRefreshFailed" : "sidebar:pageLoadFailed"),
      canRetry: true,
    };
  }
  return { message: validationMessage(error.body, t), canRetry: false };
}

function boundedInteger(value: unknown, min: number, max: number): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= min && value <= max;
}

function validationMessage(body: unknown, t: TFunction): string {
  const details = isRecord(body) && isRecord(body.details) ? body.details : {};
  const index = details.filter_index;
  const limit = details.limit;
  const hasIndex = boundedInteger(index, 0, 19);
  const hasLimit = boundedInteger(limit, 1, 262144);
  const values = { filter: hasIndex ? index + 1 : 0, limit: hasLimit ? limit : 0 };
  if (hasIndex && hasLimit && details.reason === "list_count") {
    return t("sidebar:filterListLimit", values);
  }
  if (hasIndex && hasLimit && details.reason === "scalar_length") {
    return t("sidebar:filterValueLimit", values);
  }
  if (hasLimit && details.reason === "clause_count") {
    return t("sidebar:filterCountLimit", values);
  }
  if (hasIndex && details.reason === "invalid_clause") {
    return t("sidebar:filterInvalid", values);
  }
  return t("sidebar:queryInvalid");
}
