import { useQuery } from "@tanstack/react-query";
import { useSystemInfoBootId } from "@/components/system-info-query-provider";
import { fetchSessionCapacitySettings } from "@/lib/api/domains/session-capacity-api";
import {
  createSystemInfoQueryKey,
  SESSION_CAPACITY_QUERY_KEY_PREFIX,
  useSystemInfoQueryIdentity,
} from "./system-info-query";

export function useEffectiveSessionCapacity() {
  const identity = useSystemInfoQueryIdentity(useSystemInfoBootId());
  const capacity = useQuery({
    queryKey: [
      ...SESSION_CAPACITY_QUERY_KEY_PREFIX,
      ...createSystemInfoQueryKey(identity).slice(2),
    ],
    queryFn: ({ signal }) =>
      fetchSessionCapacitySettings({ baseUrl: identity.apiBaseUrl, init: { signal } }),
    refetchOnMount: "always",
    refetchInterval: 10_000,
    retry: false,
  });
  return capacity.isSuccess ? capacity.data.effective : undefined;
}
