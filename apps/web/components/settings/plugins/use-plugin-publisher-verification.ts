"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { PluginRecord } from "@/lib/types/plugins";
import { ApiError } from "@/lib/api/client";

type VerifyPublisherAction = (
  id: string,
  expectedInstallationID: string,
  expectedVersion: string,
) => Promise<boolean>;

export function usePluginPublisherVerification(
  plugin: PluginRecord | null,
  verifyPublisher: VerifyPublisherAction,
) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [success, setSuccess] = useState(false);
  const mountedRef = useRef(true);
  const requestGeneration = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);
  useEffect(() => {
    requestGeneration.current += 1;
    setBusy(false);
    setError(undefined);
    setSuccess(false);
  }, [plugin?.id, plugin?.installation_id, plugin?.version]);

  const verify = async () => {
    if (!plugin?.installation_id) return;
    const requestID = ++requestGeneration.current;
    const expectedInstallationID = plugin.installation_id;
    const expectedVersion = plugin.version;
    setBusy(true);
    setError(undefined);
    setSuccess(false);
    try {
      const applied = await verifyPublisher(plugin.id, expectedInstallationID, expectedVersion);
      if (mountedRef.current && requestID === requestGeneration.current && applied) {
        setSuccess(true);
      }
    } catch (reason) {
      if (mountedRef.current && requestID === requestGeneration.current) {
        setError(publisherVerificationMessage(reason, t));
      }
    } finally {
      if (mountedRef.current && requestID === requestGeneration.current) {
        setBusy(false);
      }
    }
  };

  return { busy, error, success, verify };
}

function publisherVerificationMessage(reason: unknown, t: (key: string) => string): string {
  const code = reason instanceof ApiError ? (reason.errorCode ?? "") : "";
  switch (code) {
    case "publisher_evidence_unavailable":
    case "catalog_unavailable":
      return t("plugins:publisherVerificationUnavailable");
    case "installed_package_mismatch":
    case "package_identity_mismatch":
      return t("plugins:publisherVerificationMismatch");
    case "installed_package_unreadable":
      return t("plugins:publisherVerificationUnreadable");
    case "installed_package_changed":
    case "publisher_changed":
      return t("plugins:publisherVerificationChanged");
    default:
      return t("plugins:publisherVerificationFailed");
  }
}
