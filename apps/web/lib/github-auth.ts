import type { GitHubStatus } from "@/lib/types/github";

export function hasGitHubPersonalActor(status: GitHubStatus | null): boolean {
  const actor = status?.effective_personal_actor;
  return Boolean(status?.authenticated && actor?.kind === "human" && actor.login);
}

export type GitHubPersonalIdentityState = {
  active: boolean;
  actor: string;
  personalOAuthActive: boolean;
};

export function getGitHubPersonalIdentityState(status: GitHubStatus): GitHubPersonalIdentityState {
  const active = hasGitHubPersonalActor(status);
  const principal = status.effective_personal_actor;
  const personalOAuthActive = Boolean(
    active && principal?.source === "github_app_user" && status.personal?.status === "active",
  );
  let actor = "Unavailable";
  if (active) actor = principal?.login ?? "Unavailable";
  else if (status.automation?.source === "github_app_installation") actor = "Not connected";
  return { active, actor, personalOAuthActive };
}

export function getGitHubMutationActor(status: GitHubStatus | null) {
  if (!status?.authenticated) return null;
  const effective = status?.effective_manual_mutation_actor;
  if (!effective) return null;
  if (effective.kind === "app") {
    return effective.login ? `${effective.login} GitHub App` : "GitHub App";
  }
  return effective.login ?? null;
}
