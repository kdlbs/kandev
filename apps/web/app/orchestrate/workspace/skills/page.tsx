import { listSkills } from "@/lib/api/domains/orchestrate-api";
import { getActiveWorkspaceId } from "../../lib/get-active-workspace";
import { SkillsPageClient } from "./skills-page-client";
import type { Skill } from "@/lib/state/slices/orchestrate/types";

export default async function SkillsPage() {
  const workspaceId = await getActiveWorkspaceId();

  let skills: Skill[] = [];
  if (workspaceId) {
    const res = await listSkills(workspaceId, { cache: "no-store" }).catch(() => ({
      skills: [],
    }));
    skills = res.skills ?? [];
  }

  return <SkillsPageClient initialSkills={skills} />;
}
