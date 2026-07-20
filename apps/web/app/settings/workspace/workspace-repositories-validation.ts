import type { RepositoryPathValidationResponse } from "@/lib/types/http";

export function isValidManualRepository(result: RepositoryPathValidationResponse): boolean {
  return result.exists && result.is_git;
}
