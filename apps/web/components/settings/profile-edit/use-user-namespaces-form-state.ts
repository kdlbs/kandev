import { useCallback, useState } from "react";

export function useUserNamespacesFormState(config?: Record<string, string>) {
  const baselineEnabled = config?.allow_user_namespaces === "true";
  const [allowUserNamespaces, setAllowUserNamespaces] = useState(baselineEnabled);
  const resetUserNamespaces = useCallback(() => {
    setAllowUserNamespaces(baselineEnabled);
  }, [baselineEnabled]);

  return { allowUserNamespaces, setAllowUserNamespaces, resetUserNamespaces };
}
