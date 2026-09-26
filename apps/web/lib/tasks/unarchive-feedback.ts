import type { UnarchiveTaskResponse } from "@/lib/api/domains/kanban-api";
import { t } from "@/lib/i18n";

export type UnarchiveToastPayload = {
  title: string;
  description: string;
  variant?: "success";
};

// Builds the toast shown after a successful unarchive. When the archived
// branch no longer exists anywhere (never pushed, local copy deleted by
// archive), the prior work is unrecoverable and the user must be told the
// next session starts fresh.
//
// The missing-branch sentence used to inflect itself
// (`${plural ? "Branches" : "Branch"} … no longer ${plural ? "exist" : "exists"}`),
// which puts the plural rule at the call site and cannot be translated into a
// language whose rules differ from English. `count` plus `_one`/`_other` moves
// it into the catalog, where each locale supplies as many forms as it needs.
export function unarchiveToastPayload(result: UnarchiveTaskResponse): UnarchiveToastPayload {
  const missing = (result.recovery ?? []).filter((r) => r.status === "missing");
  if (missing.length > 0) {
    return {
      title: t("tasks:taskUnarchived"),
      description: t("tasks:unarchivedBranchesMissing", {
        count: missing.length,
        branches: missing.map((r) => r.branch).join(", "),
      }),
    };
  }
  return {
    title: t("tasks:taskUnarchived"),
    description: t("tasks:unarchivedTaskRestored"),
    variant: "success",
  };
}
