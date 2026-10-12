import type { ImmerSet } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

export function buildSessionGitCheckoutActions(
  set: ImmerSet,
): Pick<SessionRuntimeSlice, "bumpSessionGitCheckoutGeneration"> {
  return {
    bumpSessionGitCheckoutGeneration: (sessionId, repositoryName) =>
      set((draft) => {
        const envKey = draft.environmentIdBySessionId[sessionId] ?? sessionId;
        const byRepository = (draft.gitCheckoutGeneration.byEnvironmentId[envKey] ??= {});
        const scope = repositoryName ?? "";
        byRepository[scope] = (byRepository[scope] ?? 0) + 1;

        const rawByRepository = draft.gitStatus.byEnvironmentRepo[envKey];
        if (rawByRepository) {
          delete rawByRepository[scope];
          if (Object.keys(rawByRepository).length === 0) {
            delete draft.gitStatus.byEnvironmentRepo[envKey];
          }
        }
        const latestRawStatus = draft.gitStatus.byEnvironmentId[envKey];
        if (
          repositoryName === undefined ||
          (latestRawStatus?.repository_name ?? "") === repositoryName
        ) {
          delete draft.gitStatus.byEnvironmentId[envKey];
        }

        const displayByRepository = draft.gitStatusDisplay.byEnvironmentRepo[envKey];
        if (displayByRepository) {
          delete displayByRepository[scope];
          if (Object.keys(displayByRepository).length === 0) {
            delete draft.gitStatusDisplay.byEnvironmentRepo[envKey];
          }
        }
      }),
  };
}
