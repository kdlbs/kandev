import type {
  RepositoryInspection,
  RepositoryProviderListResult,
  RepositoryProviderPage,
  RepositoryProviderRegistration,
  ReviewProviderRegistration,
} from "./types";
import { normalizeReviewAssociations, normalizeReviewItems } from "./registry-normalization";

type RunAbortable = <T>(
  signal: AbortSignal,
  operation: (signal: AbortSignal) => Promise<T>,
) => Promise<T>;

export function wrapRepositoryProviderLifecycle(
  provider: RepositoryProviderRegistration,
  runAbortable: RunAbortable,
): RepositoryProviderRegistration {
  const wrapped: RepositoryProviderRegistration = {
    ...provider,
    listRepositories: ({ workspaceId, query, cursor, limit, signal }) =>
      runAbortable(signal, (lifecycleSignal) =>
        provider.listRepositories({
          workspaceId,
          query,
          cursor,
          limit,
          signal: lifecycleSignal,
        }),
      ).then((result) => bindRepositoryListResult(provider.id, result)),
    listBranches: ({ workspaceId, repository, signal }) =>
      runAbortable(signal, (lifecycleSignal) =>
        provider.listBranches({ workspaceId, repository, signal: lifecycleSignal }),
      ),
    inspectURL: ({ workspaceId, url, signal }) =>
      runAbortable(signal, (lifecycleSignal) =>
        provider.inspectURL({ workspaceId, url, signal: lifecycleSignal }),
      ).then((repository) => (repository ? bindRepositoryProvider(provider.id, repository) : null)),
  };
  if (provider.createChangeRequest) {
    const createChangeRequest = provider.createChangeRequest;
    wrapped.createChangeRequest = (context) =>
      runAbortable(context.signal, (lifecycleSignal) =>
        createChangeRequest({ ...context, signal: lifecycleSignal }),
      );
  }
  return wrapped;
}

function bindRepositoryListResult(
  providerId: string,
  result: RepositoryProviderListResult,
): RepositoryProviderPage | RepositoryInspection[] {
  if (Array.isArray(result)) {
    return result.map((repository) => bindRepositoryProvider(providerId, repository));
  }
  return {
    ...result,
    repositories: result.repositories.map((repository) =>
      bindRepositoryProvider(providerId, repository),
    ),
  };
}

/** Provider identity is registration-owned, never trusted from plugin result data. */
function bindRepositoryProvider(
  providerId: string,
  repository: RepositoryInspection,
): RepositoryInspection {
  return { ...repository, providerId };
}

export function wrapReviewProviderLifecycle(
  provider: ReviewProviderRegistration,
  runAbortable: RunAbortable,
  trackSubscription: (unsubscribe: () => void) => () => void,
): ReviewProviderRegistration {
  const wrapped: ReviewProviderRegistration = {
    ...provider,
    getSnapshot: (taskId) => normalizeReviewItems(provider.id, provider.getSnapshot(taskId)),
    subscribe: (taskId, listener) => trackSubscription(provider.subscribe(taskId, listener)),
    refresh: (taskId, signal) =>
      runAbortable(signal, (lifecycleSignal) => provider.refresh(taskId, lifecycleSignal)),
  };
  if (provider.getAssociationSnapshot) {
    const getAssociationSnapshot = provider.getAssociationSnapshot;
    wrapped.getAssociationSnapshot = (workspaceId) =>
      normalizeReviewAssociations(provider.id, getAssociationSnapshot(workspaceId));
  }
  if (provider.subscribeAssociations) {
    const subscribeAssociations = provider.subscribeAssociations;
    wrapped.subscribeAssociations = (workspaceId, listener) =>
      trackSubscription(subscribeAssociations(workspaceId, listener));
  }
  if (provider.refreshAssociations) {
    const refreshAssociations = provider.refreshAssociations;
    wrapped.refreshAssociations = (workspaceId, signal) =>
      runAbortable(signal, (lifecycleSignal) => refreshAssociations(workspaceId, lifecycleSignal));
  }
  if (provider.unlink) {
    const unlink = provider.unlink;
    wrapped.unlink = (context) =>
      runAbortable(context.signal, (lifecycleSignal) =>
        unlink({ ...context, signal: lifecycleSignal }),
      );
  }
  return wrapped;
}
