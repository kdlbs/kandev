import { redirect } from "next/navigation";
import { listWorkspaces } from "@/lib/api";

export default async function GeneralGitHubPage() {
  const workspaces = await listWorkspaces({ cache: "no-store" }).catch(() => ({
    workspaces: [],
  }));

  const firstWorkspace = workspaces.workspaces[0];
  if (firstWorkspace) {
    redirect(`/settings/workspace/${firstWorkspace.id}/github`);
  }

  redirect("/settings/workspace");
}
