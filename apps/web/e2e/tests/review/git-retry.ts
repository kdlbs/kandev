import { dwell } from "../../helpers/causal-waits";

const GIT_INDEX_LOCK_ATTEMPTS = 3;
const GIT_INDEX_LOCK_RETRY_MS = 300;

/** Retries the short-lived index lock taken by the backend's Git status refresh. */
export async function retryGitIndexLock<T>(operation: () => T): Promise<T> {
  for (let attempt = 0; attempt < GIT_INDEX_LOCK_ATTEMPTS; attempt++) {
    try {
      return operation();
    } catch (error) {
      const isLastAttempt = attempt === GIT_INDEX_LOCK_ATTEMPTS - 1;
      if (!(error instanceof Error) || !error.message.includes("index.lock") || isLastAttempt) {
        throw error;
      }
      await dwell(
        GIT_INDEX_LOCK_RETRY_MS,
        "poll-interval",
        "the backend's periodic Git status refresh can briefly hold the submodule index lock without publishing a completion event",
      );
    }
  }
  throw new Error("Git index lock retry exhausted");
}
