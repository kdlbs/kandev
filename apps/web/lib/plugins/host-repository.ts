import type { PluginHostRepository } from "./types";
import type { Repository } from "@/lib/types/http";

/** Projects a persisted repository into the stable read-only plugin contract. */
export function toPluginHostRepository(repository: Repository): PluginHostRepository {
  return {
    id: repository.id,
    workspace_id: repository.workspace_id,
    name: repository.name,
    provider: repository.provider,
    source_type: repository.source_type,
    provider_repo_id: repository.provider_repo_id,
    ...(repository.provider_host ? { provider_host: repository.provider_host } : {}),
    ...(repository.provider_scope ? { provider_scope: repository.provider_scope } : {}),
    ...(repository.provider_owner ? { provider_owner: repository.provider_owner } : {}),
    ...(repository.provider_name ? { provider_name: repository.provider_name } : {}),
    ...(repository.remote_url ? { remote_url: repository.remote_url } : {}),
    ...(repository.default_branch ? { default_branch: repository.default_branch } : {}),
  };
}
