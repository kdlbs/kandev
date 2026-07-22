import { describe, expect, it } from "vitest";
import { resolveExternalVcsFileURL, type ExternalVcsFileURLInput } from "./external-vcs-file-url";

const githubRepository = {
  provider: "github",
  provider_host: "https://github.com",
  provider_owner: "acme",
  provider_name: "web",
  remote_url: "https://github.com/acme/web.git",
};
const renamedPath = "src/new.ts";
const previousRenamedPath = "src/old.ts";
const gitlabOwner = "platform/tools";
const currentPath = "src/app.ts";

function resolve(overrides: Partial<ExternalVcsFileURLInput> = {}) {
  return resolveExternalVcsFileURL({
    repository: githubRepository,
    path: currentPath,
    baseBranch: "main",
    ...overrides,
  });
}

describe("resolveExternalVcsFileURL provider routes", () => {
  it.each([
    {
      name: "GitHub",
      repository: githubRepository,
      expected: "https://github.com/acme/web/blob/feature%2Fshare/src/a%20file.ts",
    },
    {
      name: "self-hosted GitLab",
      repository: {
        provider: "gitlab",
        provider_host: "https://gitlab.example.com:8443",
        provider_owner: gitlabOwner,
        provider_name: "api",
        remote_url: "https://gitlab.example.com:8443/platform/tools/api.git",
      },
      expected:
        "https://gitlab.example.com:8443/platform/tools/api/-/blob/feature%2Fshare/src/a%20file.ts",
    },
    {
      name: "Azure DevOps",
      repository: {
        provider: "azure_devops",
        provider_host: "",
        provider_owner: "Platform",
        provider_name: "api",
        remote_url: "https://dev.azure.com/acme/Platform/_git/api",
      },
      expected:
        "https://dev.azure.com/acme/Platform/_git/api?path=%2Fsrc%2Fa+file.ts&version=GBfeature%2Fshare",
    },
  ])("builds the $name URL from credential-free metadata", ({ repository, expected }) => {
    expect(
      resolve({ repository, path: "src/a file.ts", publishedBranch: "feature/share" }),
    ).toEqual({
      provider: repository.provider,
      url: expected,
      path: "src/a file.ts",
      revision: "feature/share",
    });
  });

  it("encodes every path segment without turning a filename into a route", () => {
    expect(resolve({ path: "docs/a#b?/100%.md", publishedBranch: "release #1" })?.url).toBe(
      "https://github.com/acme/web/blob/release%20%231/docs/a%23b%3F/100%25.md",
    );
  });

  it.each([
    {
      name: "GitHub SCP clone identity",
      repository: { ...githubRepository, remote_url: "git@github.com:acme/web.git" },
      expected: "https://github.com/acme/web/blob/feature%2Fshare/src/app.ts",
    },
    {
      name: "GitHub SSH URL clone identity",
      repository: { ...githubRepository, remote_url: "ssh://git@github.com/acme/web.git" },
      expected: "https://github.com/acme/web/blob/feature%2Fshare/src/app.ts",
    },
    {
      name: "self-hosted GitLab SCP clone identity",
      repository: {
        provider: "gitlab",
        provider_host: "https://gitlab.example.com",
        provider_owner: gitlabOwner,
        provider_name: "api",
        remote_url: "git@gitlab.example.com:platform/tools/api.git",
      },
      expected: "https://gitlab.example.com/platform/tools/api/-/blob/feature%2Fshare/src/app.ts",
    },
    {
      name: "self-hosted GitLab SSH URL with a custom SSH port",
      repository: {
        provider: "gitlab",
        provider_host: "https://gitlab.example.com:8443",
        provider_owner: gitlabOwner,
        provider_name: "api",
        remote_url: "ssh://git@gitlab.example.com:2222/platform/tools/api.git",
      },
      expected:
        "https://gitlab.example.com:8443/platform/tools/api/-/blob/feature%2Fshare/src/app.ts",
    },
    {
      name: "Azure DevOps SCP clone identity",
      repository: {
        provider: "azure_devops",
        provider_host: "",
        provider_owner: "Platform",
        provider_name: "api",
        remote_url: "git@ssh.dev.azure.com:v3/acme/Platform/api",
      },
      expected:
        "https://dev.azure.com/acme/Platform/_git/api?path=%2Fsrc%2Fapp.ts&version=GBfeature%2Fshare",
    },
  ])("builds a credential-free HTTPS URL from a $name", ({ repository, expected }) => {
    expect(resolve({ repository, publishedBranch: "feature/share" })?.url).toBe(expected);
  });
});

describe("resolveExternalVcsFileURL revision and path selection", () => {
  it("prefers a published review branch for an existing file", () => {
    expect(resolve({ publishedBranch: "feature/published" })?.revision).toBe("feature/published");
  });

  it.each(["added", "untracked"] as const)(
    "omits an %s file when only the base branch is known",
    (status) => {
      expect(resolve({ status })).toBeNull();
    },
  );

  it("targets a deleted file on the base branch", () => {
    expect(resolve({ status: "deleted", publishedBranch: "feature/deleted" })).toMatchObject({
      revision: "main",
      path: currentPath,
    });
  });

  it("targets a renamed file's new path on a published branch", () => {
    expect(
      resolve({
        status: "renamed",
        path: renamedPath,
        previousPath: previousRenamedPath,
        publishedBranch: "feature/rename",
      }),
    ).toMatchObject({ revision: "feature/rename", path: renamedPath });
  });

  it("targets a renamed file's previous path on the base branch", () => {
    expect(
      resolve({ status: "renamed", path: renamedPath, previousPath: previousRenamedPath }),
    ).toMatchObject({ revision: "main", path: previousRenamedPath });
  });

  it("omits a base-only rename when the previous path is unknown", () => {
    expect(resolve({ status: "renamed", path: renamedPath })).toBeNull();
  });

  it("preserves leading and trailing spaces in the current Git path", () => {
    expect(resolve({ path: " src/app.ts " })).toMatchObject({
      path: " src/app.ts ",
      url: "https://github.com/acme/web/blob/main/%20src/app.ts%20",
    });
  });

  it("preserves leading and trailing spaces in a renamed file's previous Git path", () => {
    expect(
      resolve({
        status: "renamed",
        path: renamedPath,
        previousPath: " src/old.ts ",
      }),
    ).toMatchObject({
      path: " src/old.ts ",
      url: "https://github.com/acme/web/blob/main/%20src/old.ts%20",
    });
  });
});

describe("resolveExternalVcsFileURL security and completeness", () => {
  it.each([
    ["non-HTTPS remote", { remote_url: "http://github.com/acme/web.git" }],
    ["embedded username", { remote_url: "https://alice@github.com/acme/web.git" }],
    ["embedded password", { remote_url: "https://alice:secret@github.com/acme/web.git" }],
    ["unsupported provider", { provider: "bitbucket" }],
    ["mismatched repository path", { remote_url: "https://github.com/other/web.git" }],
    ["missing provider owner", { provider_owner: "" }],
  ])("fails closed for a %s", (_name, repositoryOverrides) => {
    expect(resolve({ repository: { ...githubRepository, ...repositoryOverrides } })).toBeNull();
  });

  it.each(["/etc/passwd", "../secret.txt", "src/../../secret.txt", "C:\\secret.txt"])(
    "does not expose a local or escaping path: %s",
    (path) => {
      expect(resolve({ path })).toBeNull();
    },
  );

  it("requires the persisted GitLab provider host to match the remote origin", () => {
    expect(
      resolve({
        repository: {
          provider: "gitlab",
          provider_host: "https://gitlab.internal.example",
          provider_owner: "acme",
          provider_name: "web",
          remote_url: "https://gitlab.example.com/acme/web.git",
        },
      }),
    ).toBeNull();
  });

  it.each([
    "javascript:alert(1)",
    "data:text/plain,repository",
    "blob:https://github.com/id",
    "file:///tmp/repository",
    "//github.com/acme/web.git",
    "https://github.com/acme/web.git?token=secret",
    "https://github.com/acme/web.git#fragment",
    "https://github.com/acme/web.git\u0000suffix",
  ])("rejects an unsafe remote URL: %s", (remote_url) => {
    expect(resolve({ repository: { ...githubRepository, remote_url } })).toBeNull();
  });
});

describe("resolveExternalVcsFileURL literal and unsafe file paths", () => {
  it.each([
    ["src/%2e%2e/secret.ts", "src/%252e%252e/secret.ts"],
    ["src/%252e%252e/secret.ts", "src/%25252e%25252e/secret.ts"],
    ["src/%2Fsecret.ts", "src/%252Fsecret.ts"],
    ["src/%255csecret.ts", "src/%25255csecret.ts"],
  ])("treats percent-escape-looking text as a literal filename: %s", (path, encodedPath) => {
    expect(resolve({ path })?.url).toBe(`https://github.com/acme/web/blob/main/${encodedPath}`);
  });

  it("rejects a control character in a file path", () => {
    expect(resolve({ path: "src/control\u0000.ts" })).toBeNull();
  });
});

describe("resolveExternalVcsFileURL SSH identity validation", () => {
  it.each([
    "git@github.com/acme/web.git",
    "git@github.com:other/web.git",
    "alice@github.com:acme/web.git",
    "git@github.com:acme/web.git?token=secret",
    "ssh://git:secret@github.com/acme/web.git",
    "ssh://git@github.com:22/acme/web.git#fragment",
    "git@github.com:acme/%252e%252e.git",
    "git@github.example.com:acme/web.git",
  ])("rejects a malformed or mismatched GitHub SSH identity: %s", (remote_url) => {
    expect(resolve({ repository: { ...githubRepository, remote_url } })).toBeNull();
  });

  it("rejects a self-hosted GitLab SSH host that differs from provider metadata", () => {
    expect(
      resolve({
        repository: {
          provider: "gitlab",
          provider_host: "https://gitlab.example.com",
          provider_owner: gitlabOwner,
          provider_name: "api",
          remote_url: "git@gitlab.attacker.example:platform/tools/api.git",
        },
      }),
    ).toBeNull();
  });

  it.each([
    "ssh://git@gitlab.example.com:/platform/tools/api.git",
    "ssh://git@gitlab.example.com:0/platform/tools/api.git",
    "ssh://git@gitlab.example.com:65536/platform/tools/api.git",
  ])("rejects a malformed GitLab SSH port: %s", (remote_url) => {
    expect(
      resolve({
        repository: {
          provider: "gitlab",
          provider_host: "https://gitlab.example.com",
          provider_owner: gitlabOwner,
          provider_name: "api",
          remote_url,
        },
      }),
    ).toBeNull();
  });

  it("rejects an Azure DevOps SSH identity with mismatched provider metadata", () => {
    expect(
      resolve({
        repository: {
          provider: "azure_devops",
          provider_host: "",
          provider_owner: "OtherProject",
          provider_name: "api",
          remote_url: "git@ssh.dev.azure.com:v3/acme/Platform/api",
        },
      }),
    ).toBeNull();
  });
});
