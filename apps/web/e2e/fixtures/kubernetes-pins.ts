export const KIND_VERSION = "v0.32.0";
export const KUBERNETES_FIXTURE_PINS = {
  "v1.34.8": {
    kubectlSha256Amd64: "f6249132865c13abe3c9dd5038f5da65849cb86eee1608c001831504e481aa8c",
    nodeImage:
      "kindest/node:v1.34.8@sha256:02722c2dedddcfc00febf5d27fbeb9b7b2c14294c82109ff4a85d89ac9ba3256",
  },
  "v1.36.1": {
    kubectlSha256Amd64: "629d3f410e09bf49b64ae7079f7f0bda1191efed311f7d37fdbab0ad5b0ec2b7",
    nodeImage:
      "kindest/node:v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5",
  },
} as const;

export type KubernetesFixtureVersion = keyof typeof KUBERNETES_FIXTURE_PINS;

export const KUBERNETES_VERSION: KubernetesFixtureVersion = "v1.36.1";
export const KIND_NODE_IMAGE = KUBERNETES_FIXTURE_PINS[KUBERNETES_VERSION].nodeImage;
export const KUBERNETES_E2E_BASE_IMAGE =
  "ubuntu:24.04@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517";
export const KIND_SHA256_AMD64 = "50030de23cf40a18505f20426f6a8506bedf13c6e509244bd1fa9463721b0f54";
export const KUBECTL_SHA256_AMD64 = KUBERNETES_FIXTURE_PINS[KUBERNETES_VERSION].kubectlSha256Amd64;

export type KubernetesFixturePin = {
  kubectlSha256Amd64: string;
  nodeImage: string;
  version: KubernetesFixtureVersion;
};

export function resolveKubernetesFixturePin(
  configuredVersion = process.env.KANDEV_E2E_KUBERNETES_VERSION,
): KubernetesFixturePin {
  const version = configuredVersion?.trim() || KUBERNETES_VERSION;
  if (!(version in KUBERNETES_FIXTURE_PINS)) {
    throw new Error(
      `unsupported KANDEV_E2E_KUBERNETES_VERSION ${JSON.stringify(version)}; expected ${Object.keys(KUBERNETES_FIXTURE_PINS).join(" or ")}`,
    );
  }
  const supportedVersion = version as KubernetesFixtureVersion;
  return {
    version: supportedVersion,
    ...KUBERNETES_FIXTURE_PINS[supportedVersion],
  };
}
