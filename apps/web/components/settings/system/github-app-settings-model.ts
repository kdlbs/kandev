import type { StartDeploymentGitHubAppRequest } from "@/lib/types/github";

export type DeploymentAppOwnerChoice = StartDeploymentGitHubAppRequest["owner_type"];

export type DeploymentAppSetupInput = {
  ownerType: DeploymentAppOwnerChoice;
  ownerLogin: string;
  publicBaseUrl: string;
};

export type DeploymentAppSetupErrors = Partial<Record<"ownerLogin" | "publicBaseUrl", string>>;

export type DeploymentAppSetupValidation = {
  values: StartDeploymentGitHubAppRequest | null;
  errors: DeploymentAppSetupErrors;
};

export type DeploymentAppCallbackNotice = {
  tone: "success" | "info" | "error";
  title: string;
  description: string;
};

const githubOwnerPattern = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$/;

export function validateDeploymentAppSetup(
  input: DeploymentAppSetupInput,
): DeploymentAppSetupValidation {
  const ownerLogin = input.ownerLogin.trim();
  const errors: DeploymentAppSetupErrors = {};
  if (!githubOwnerPattern.test(ownerLogin)) {
    errors.ownerLogin =
      input.ownerType === "organization"
        ? "Enter the GitHub organization login."
        : "Enter the GitHub username.";
  }

  const publicBaseUrl = normalizePublicBaseUrl(input.publicBaseUrl, errors);
  return {
    values:
      Object.keys(errors).length === 0 && publicBaseUrl
        ? {
            owner_type: input.ownerType,
            owner_login: ownerLogin,
            public_base_url: publicBaseUrl,
          }
        : null,
    errors,
  };
}

function normalizePublicBaseUrl(rawValue: string, errors: DeploymentAppSetupErrors): string | null {
  let url: URL;
  try {
    url = new URL(rawValue.trim());
  } catch {
    errors.publicBaseUrl = "Enter a public HTTPS origin.";
    return null;
  }
  if (url.protocol !== "https:" || url.username || url.password) {
    errors.publicBaseUrl = "Enter a public HTTPS origin.";
    return null;
  }
  if (url.hostname === "localhost" || url.hostname.endsWith(".localhost")) {
    errors.publicBaseUrl = "Enter a public host, not localhost.";
    return null;
  }
  if ((url.pathname && url.pathname !== "/") || url.search || url.hash) {
    errors.publicBaseUrl = "Enter an origin without a path, query, or fragment.";
    return null;
  }
  return url.origin;
}

export function manifestFormFields(manifest: unknown): { manifest: string } {
  return { manifest: JSON.stringify(manifest) };
}

export function callbackNoticeForResult(result: string | null): DeploymentAppCallbackNotice | null {
  if (!result) return null;
  if (result === "connected") {
    return {
      tone: "success",
      title: "GitHub App created",
      description:
        "The deployment App is ready. Webhook health will verify after GitHub sends a signed delivery.",
    };
  }
  if (result === "github_app_registration_cancelled") {
    return {
      tone: "info",
      title: "GitHub App setup cancelled",
      description: "No App credentials were changed. Start setup again when you are ready.",
    };
  }
  if (result === "github_app_environment_read_only") {
    return {
      tone: "error",
      title: "GitHub App setup is managed externally",
      description:
        "Environment configuration has priority and cannot be replaced from System Settings.",
    };
  }
  if (result === "github_app_invalid_callback" || result.startsWith("manifest_conversion_")) {
    return {
      tone: "error",
      title: "GitHub App setup was not completed",
      description:
        "The GitHub response was expired, invalid, or could not be verified. Start setup again.",
    };
  }
  return {
    tone: "error",
    title: "GitHub App setup failed",
    description: "GitHub App setup could not be completed. Review the status and start again.",
  };
}
