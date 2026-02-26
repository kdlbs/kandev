import { useCallback, useState } from "react";
import {
  launchSession,
  type LaunchSessionRequest,
  type LaunchSessionResponse,
} from "@/lib/services/session-launch-service";

export function useSessionLaunch(options?: {
  onSuccess?: (resp: LaunchSessionResponse) => void;
  onError?: (err: Error) => void;
}): {
  launch: (request: LaunchSessionRequest) => Promise<LaunchSessionResponse | null>;
  isLoading: boolean;
  error: string | null;
} {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const launch = useCallback(
    async (request: LaunchSessionRequest): Promise<LaunchSessionResponse | null> => {
      setIsLoading(true);
      setError(null);
      try {
        const resp = await launchSession(request);
        options?.onSuccess?.(resp);
        return resp;
      } catch (err) {
        const message = err instanceof Error ? err.message : "Unknown error";
        setError(message);
        options?.onError?.(err instanceof Error ? err : new Error(message));
        return null;
      } finally {
        setIsLoading(false);
      }
    },
    [options?.onSuccess, options?.onError], // eslint-disable-line react-hooks/exhaustive-deps
  );

  return { launch, isLoading, error };
}
