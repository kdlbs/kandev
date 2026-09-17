import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import Link from "@/components/routing/app-link";
export function OrchestrationGate({ children }: { children: ReactNode }) {
  const enabled = useFeature("orchestration");
  const { t } = useTranslation();
  return enabled ? (
    <>{children}</>
  ) : (
    <p>
      {t("orchestration:orchestrationDisabled")}{" "}
      <Link className="underline" href="/settings/system/feature-toggles">
        {t("orchestration:orchestrationEnable")}
      </Link>
    </p>
  );
}
