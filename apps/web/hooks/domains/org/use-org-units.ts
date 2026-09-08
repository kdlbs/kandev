"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import {
  createUnit,
  deleteUnit,
  listUnits,
  removeUnitMember,
  renameUnit,
  setUnitMember,
  withDepth,
  type OrgUnit,
} from "@/lib/api/domains/org-units-api";

export type UnitRow = OrgUnit & { depth: number };

/**
 * The organization tree and the operations on it.
 *
 * Every refusal from the server names its blocking condition, so failures are
 * surfaced verbatim rather than replaced with a generic message: "this unit
 * still holds child units or workspaces" tells someone what to do next, and
 * "something went wrong" does not.
 */
export function useOrgUnits() {
  const { t } = useTranslation();
  const [units, setUnits] = useState<UnitRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const res = await listUnits();
      setUnits(withDepth(res.units ?? []));
    } catch {
      setUnits([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const run = useCallback(
    async (action: () => Promise<unknown>) => {
      setBusy(true);
      try {
        await action();
        await refresh();
        return true;
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t("settings:unitActionFailed"));
        return false;
      } finally {
        setBusy(false);
      }
    },
    [refresh, t],
  );

  return {
    units,
    loading,
    busy,
    refresh,
    create: (parentId: string, name: string) => run(() => createUnit(parentId, name)),
    rename: (unitId: string, name: string) => run(() => renameUnit(unitId, name)),
    remove: (unitId: string) => run(() => deleteUnit(unitId)),
    addMember: (unitId: string, userId: string, role: string) =>
      run(() => setUnitMember(unitId, userId, role)),
    removeMember: (unitId: string, userId: string) => run(() => removeUnitMember(unitId, userId)),
  };
}
