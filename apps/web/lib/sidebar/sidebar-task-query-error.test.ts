import { expect, it } from "vitest";
import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";
import { sidebarTaskQueryError } from "./sidebar-task-query-error";
const t = ((key: string, values?: unknown) => JSON.stringify({ key, values })) as TFunction;
const privateDetail = "private SQL";
const invalid = (details: unknown) =>
  new ApiError(privateDetail, 400, { error_code: "sidebar_query_invalid", details });
it.each(["list_count", "scalar_length", "invalid_clause", "clause_count"])(
  "localizes %s with safe metadata",
  (reason) => {
    const result = sidebarTaskQueryError(
      invalid({ reason, filter_index: 0, limit: 1000 }),
      false,
      t,
    );
    expect(result.canRetry).toBe(false);
    expect(result.message).not.toContain(privateDetail);
    expect(result.message).toContain('"filter":1');
  },
);
it("does not render arbitrary server strings or invalid numeric metadata", () => {
  for (const details of [
    null,
    [],
    { reason: privateDetail, filter_index: -2, limit: privateDetail },
    { reason: "list_count", filter_index: 20, limit: 1000 },
  ]) {
    expect(sidebarTaskQueryError(invalid(details), false, t)).toEqual({
      message: t("sidebar:queryInvalid"),
      canRetry: false,
    });
  }
});
it("separates initial failure, refresh failure and denied access", () => {
  expect(sidebarTaskQueryError(new Error("private"), false, t)).toEqual({
    message: t("sidebar:pageLoadFailed"),
    canRetry: true,
  });
  expect(sidebarTaskQueryError(new Error("private"), true, t)).toEqual({
    message: t("sidebar:queryRefreshFailed"),
    canRetry: true,
  });
  for (const status of [401, 403, 404])
    expect(sidebarTaskQueryError(new ApiError("private", status, {}), true, t)).toEqual({
      message: t("sidebar:workspaceContextAccessDenied"),
      canRetry: false,
    });
});
