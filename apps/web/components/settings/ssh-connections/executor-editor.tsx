"use client";

import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { ManagedBinding, type ManagedBindingProps } from "./managed-binding";

export function SSHConnectionEditor(props: ManagedBindingProps) {
  const enabled = useFeature("sshConnectionManager");
  const admin = useIsAdmin();
  const { t } = useTranslation();
  if (!enabled || !admin) return <p>{t("settings:sshManager.unavailable")}</p>;
  return <ManagedBinding {...props} />;
}
