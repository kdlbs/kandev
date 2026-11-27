import { useTranslation } from "react-i18next";
import type { CoordinatorWatchProjects } from "@/lib/api/domains/coordinator-api";

const MAX_NAMES = 5;

type ProjectsHintProps = { projects: CoordinatorWatchProjects | undefined };

function HintLine({ text }: { text: string }) {
  return (
    <span
      className="basis-full text-xs text-muted-foreground"
      data-testid="workspace-copilot-projects"
    >
      {text}
    </span>
  );
}

/** The second line of the page chip row: which projects the coordinator watches.
 *  Omitted when the project listing failed and no names came back. */
export function ProjectsHint({ projects }: ProjectsHintProps) {
  const { t } = useTranslation();
  if (!projects) return null;
  if (projects.scope === "all") {
    return (
      <HintLine
        text={t("coordinator:copilotProjectsLine", { list: t("coordinator:copilotProjectsAll") })}
      />
    );
  }
  if (!projects.names) return null;
  const names = projects.names;
  const shown = names.slice(0, MAX_NAMES).join(", ");
  const more = names.length - MAX_NAMES;
  const parts: string[] = [];
  if (shown) parts.push(shown);
  if (more > 0) parts.push(t("coordinator:copilotProjectsMore", { count: more }));
  if (projects.include_no_repository) parts.push(t("coordinator:copilotProjectsNoRepository"));
  if (parts.length === 0) return null;
  return <HintLine text={t("coordinator:copilotProjectsLine", { list: parts.join(" ") })} />;
}
