import type { Repository } from "@/lib/types/http";

type ProjectRepositoryEligibility = Pick<
  Repository,
  "source_type" | "remote_url" | "provider_owner" | "provider_name" | "default_branch"
>;

export function isRemoteBackedProjectRepository(repository: ProjectRepositoryEligibility): boolean {
  // i18n-exempt: repository source type is a serialized enum value.
  if (
    repository.source_type.trim().toLowerCase() === "local" ||
    !repository.default_branch.trim()
  ) {
    return false;
  }
  if (repository.remote_url?.trim()) return true;
  return Boolean(repository.provider_owner.trim() && repository.provider_name.trim());
}
