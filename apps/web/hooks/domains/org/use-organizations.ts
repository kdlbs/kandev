"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import {
  createOrg,
  createOrgAdmin,
  deleteOrg,
  getCurrentOrg,
  listOrgs,
  updateOrg,
} from "@/lib/api/domains/org-api";
import type { Org } from "@/lib/types/org";

/**
 * Operator-side organization management.
 *
 * `isOperator` decides whether the surface exists at all: a non-operator gets
 * 404 from these routes, so the page must not render for them.
 */
export function useOrganizations() {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [isOperator, setIsOperator] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const current = await getCurrentOrg();
      setIsOperator(current.is_operator);
      if (!current.is_operator) {
        setOrgs([]);
        return;
      }
      setOrgs((await listOrgs()).orgs ?? []);
    } catch {
      setIsOperator(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const run = useCallback(
    async (action: () => Promise<unknown>, success: string) => {
      setBusy(true);
      try {
        await action();
        await refresh();
        toast({ title: success });
      } catch (error) {
        toast({
          title: error instanceof Error ? error.message : t("orgs:genericError"),
          variant: "error",
        });
      } finally {
        setBusy(false);
      }
    },
    [refresh, toast, t],
  );

  return {
    orgs,
    isOperator,
    busy,
    create: (name: string, onDone: () => void) =>
      void run(async () => {
        await createOrg(name);
        onDone();
      }, t("orgs:created")),
    rename: (id: string, name: string) =>
      void run(() => updateOrg(id, { name }), t("orgs:renamed")),
    suspend: (id: string) =>
      void run(() => updateOrg(id, { status: "suspended" }), t("orgs:suspended")),
    resume: (id: string) => void run(() => updateOrg(id, { status: "active" }), t("orgs:resumed")),
    remove: (id: string, slug: string, onDone: () => void) =>
      void run(async () => {
        await deleteOrg(id, slug);
        onDone();
      }, t("orgs:deleted")),
    addAdmin: (
      id: string,
      admin: { email: string; password: string; display_name: string },
      onDone: () => void,
    ) =>
      void run(async () => {
        await createOrgAdmin(id, admin);
        onDone();
      }, t("orgs:adminCreated")),
  };
}
