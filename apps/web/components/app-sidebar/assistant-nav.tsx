import { IconSparkles } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";
export function AssistantNav({
  collapsed = false,
  onNavigate,
}: {
  collapsed?: boolean;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  const enabled = useAppStore((s) => s.features.personalAssistant);
  const router = useRouter();
  if (!enabled) return null;
  return (
    <AppSidebarNavItem
      icon={IconSparkles}
      label={t("orchestration:assistant")}
      href="/assistant"
      collapsed={collapsed}
      testId="assistant-nav"
      className="max-md:min-h-11"
      onClick={
        onNavigate
          ? () => {
              router.push("/assistant");
              onNavigate();
            }
          : undefined
      }
    />
  );
}
