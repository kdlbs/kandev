"use client";

import { useTranslation } from "react-i18next";

export function SessionDeleteDescription({
  isPrimary,
  isOnlySession,
}: {
  isPrimary: boolean;
  isOnlySession: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <span>{t("task:thisWillPermanentlyDeleteTheConversation")}</span>
      {isPrimary && !isOnlySession && (
        <span className="mt-2 block font-medium">{t("task:thisIsThePrimarySessionAnother")}</span>
      )}
      {isOnlySession && (
        <span className="mt-2 block font-medium">{t("task:thisIsTheOnlySessionFor")}</span>
      )}
    </>
  );
}
