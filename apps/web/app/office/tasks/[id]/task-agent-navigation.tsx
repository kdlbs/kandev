"use client";
import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";

export function TaskAgentNavigation({
  workspaceId,
  agentId,
  parentId,
}: {
  workspaceId: string;
  agentId?: string;
  parentId?: string;
}) {
  const { t } = useTranslation();
  const query = `?workspaceId=${encodeURIComponent(workspaceId)}`;
  return (
    <nav
      className="flex flex-wrap gap-4 px-4 py-2 text-sm border-b"
      aria-label={t("office:coordinationNavigation")}
    >
      <Link
        className="underline cursor-pointer"
        href={`/settings/workspaces/${workspaceId}/agents`}
      >
        {t("office:workspaceAgents")}
      </Link>
      {agentId && (
        <Link className="underline cursor-pointer" href={`/office/agents/${agentId}${query}`}>
          {t("office:assignedAgent")}
        </Link>
      )}
      {parentId && (
        <Link className="underline cursor-pointer" href={`/office/tasks/${parentId}${query}`}>
          {t("office:parentConversationOrTask")}
        </Link>
      )}
    </nav>
  );
}
