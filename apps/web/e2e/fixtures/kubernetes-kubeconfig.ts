export type KubernetesClusterConnection = {
  server: string;
  "certificate-authority-data": string;
};

export function buildServiceAccountKubeconfig(
  cluster: KubernetesClusterConnection,
  serviceAccount: string,
  token: string,
) {
  const name = `kind-${serviceAccount}`;
  return {
    apiVersion: "v1",
    kind: "Config",
    clusters: [{ name, cluster }],
    contexts: [{ name, context: { cluster: name, user: serviceAccount } }],
    "current-context": name,
    users: [{ name: serviceAccount, user: { token } }],
  };
}
