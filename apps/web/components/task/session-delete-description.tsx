"use client";

import { useTranslation } from "react-i18next";

export function SessionDeleteDescription({
  isPrimary,
  isOnlySession,
  structured = false,
}: {
  isPrimary: boolean;
  isOnlySession: boolean;
  structured?: boolean;
}) {
  const { t } = useTranslation();
  const primaryNotice = isPrimary && !isOnlySession;

  if (structured) {
    return (
      <>
        <p>{t("task:thisWillPermanentlyDeleteTheConversation")}</p>
        {primaryNotice && <p className="font-medium">{t("task:thisIsThePrimarySessionAnother")}</p>}
        {isOnlySession && <p className="font-medium">{t("task:thisIsTheOnlySessionFor")}</p>}
      </>
    );
  }

  return (
    <>
      <span>{t("task:thisWillPermanentlyDeleteTheConversation")}</span>
      {primaryNotice && (
        <span className="mt-2 block font-medium">{t("task:thisIsThePrimarySessionAnother")}</span>
      )}
      {isOnlySession && (
        <span className="mt-2 block font-medium">{t("task:thisIsTheOnlySessionFor")}</span>
      )}
    </>
  );
}
