import type { KubernetesProfileConfigForm } from "@/components/settings/kubernetes-config";
import { kubernetesProfileInvalidReason } from "@/components/settings/kubernetes-validation";
import type { ProfileEnvVar } from "@/lib/types/http";
import { rowsToEnvVars, type EnvVarRow } from "./env-vars-card";
import {
  dockerNetworksInvalidReasonKey,
  type DockerNetworkFormValues,
} from "./build-docker-network-config";

const SPRITES_TOKEN_KEY = "SPRITES_API_TOKEN";

export function buildProfileEnvVars(
  rows: EnvVarRow[],
  isSprites: boolean,
  spritesSecretId: string | null,
): ProfileEnvVar[] {
  const vars = rowsToEnvVars(rows).filter(
    (envVar) => !isSprites || envVar.key !== SPRITES_TOKEN_KEY,
  );
  if (isSprites && spritesSecretId) {
    vars.push({ key: SPRITES_TOKEN_KEY, secret_id: spritesSecretId });
  }
  return vars;
}

type ProfileValidationState = DockerNetworkFormValues & {
  isKubernetes: boolean;
  name: string;
  mcpPolicyErrorKey: string | null;
  isSprites: boolean;
  spritesSecretId: string | null;
  kubernetesProfile: KubernetesProfileConfigForm;
};

export function profileSaveInvalidReason(
  form: ProfileValidationState,
  canManageKubernetes: boolean,
  t: (key: string) => string,
): string | undefined {
  if (form.isKubernetes && !canManageKubernetes) {
    return t("executors:kubernetesAdminSaveOnly");
  }
  if (!form.name.trim()) return t("executors:profileNameIsRequired");
  if (form.mcpPolicyErrorKey) return t(form.mcpPolicyErrorKey);
  if (form.isSprites && !form.spritesSecretId) return t("executors:spritesTokenIsRequired");
  const networkReasonKey = dockerNetworksInvalidReasonKey(form);
  if (networkReasonKey) return t(networkReasonKey);
  return form.isKubernetes ? kubernetesProfileInvalidReason(form.kubernetesProfile, t) : undefined;
}
