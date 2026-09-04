import { afterAll, describe, expect, it } from "vitest";
import { activateLocale, i18n, t } from "@/lib/i18n";
import { getExecutorLabel } from "@/lib/executor-icons";
import {
  EXECUTOR_TYPE_MAP,
  executorTypeLabel,
} from "@/app/settings/executors/new/[type]/executor-types";
import { validateMcpPolicy } from "@/components/settings/profile-edit/mcp-policy-card";
import { getExecutorDescription } from "./executor-description";

/**
 * Byte-for-byte English for the executor type labels and descriptions, as they
 * rendered before this migration.
 *
 * This exists because the copy is duplicated across THREE surfaces that had
 * already drifted from each other:
 *
 *   - `/settings/executors` (the hub cards) uses the imperative mood
 *     ("Run agents directly in the repository folder.");
 *   - `/settings/executors/new/:type` (the create header) uses the third
 *     person ("Runs agents directly in the repository folder.");
 *   - `/settings/executors/:profileId` (the edit header) uses the third person
 *     too, and shares those five sentences — but its SSH sentence is a
 *     different one again.
 *
 * Folding them onto one key set would have rewritten user-visible English on
 * two of the three, and every gate would still have passed. The identical
 * sentences share a key; the ones that differ keep their own. This table is
 * what says so after the reviewer leaves.
 *
 * De-duplicating the three tables is a behaviour change and belongs in its own
 * PR — see the guard comment in eslint.i18n.options.mjs.
 */
const SPRITES_DEV = "Sprites.dev";
const SSH = "SSH";
const KUBERNETES = "Kubernetes";

const HUB_CARD_COPY: Array<{ type: string; label: string; description: string }> = [
  { type: "local", label: "Local", description: "Run agents directly in the repository folder." },
  {
    type: "worktree",
    label: "Worktree",
    description: "Create git worktrees for isolated agent sessions.",
  },
  {
    type: "local_docker",
    label: "Docker",
    description: "Run Docker containers on this machine.",
  },
  {
    type: "sprites",
    label: SPRITES_DEV,
    description: "Run agents in Sprites.dev cloud sandboxes.",
  },
  {
    type: "ssh",
    label: SSH,
    description: "Connect to a remote host over SSH and run agentctl there.",
  },
  {
    type: "k8s",
    label: KUBERNETES,
    description: "Run agents in administrator-configured Kubernetes Pods.",
  },
];

const HUB_CARD_KEYS: Record<string, { labelKey?: string; brand?: string; descriptionKey: string }> =
  {
    local: { labelKey: "executors:typeLocal", descriptionKey: "executors:hubDescriptionLocal" },
    worktree: {
      labelKey: "executors:typeWorktree",
      descriptionKey: "executors:hubDescriptionWorktree",
    },
    local_docker: { brand: "Docker", descriptionKey: "executors:hubDescriptionDocker" },
    sprites: { brand: SPRITES_DEV, descriptionKey: "executors:hubDescriptionSprites" },
    ssh: { brand: SSH, descriptionKey: "executors:hubDescriptionSsh" },
    k8s: {
      labelKey: "executors:typeKubernetes",
      descriptionKey: "executors:hubDescriptionKubernetes",
    },
  };

describe("executors hub card copy", () => {
  it.each(HUB_CARD_COPY)("renders $type unchanged", ({ type, label, description }) => {
    const entry = HUB_CARD_KEYS[type];
    expect(entry.brand ?? t(entry.labelKey as string)).toBe(label);
    expect(t(entry.descriptionKey)).toBe(description);
  });
});

describe("create-page executor type registry", () => {
  const CREATE_COPY: Array<{ type: string; label: string; description: string }> = [
    {
      type: "local",
      label: "Local",
      description: "Runs agents directly in the repository folder.",
    },
    {
      type: "worktree",
      label: "Worktree",
      description: "Creates git worktrees for isolated agent sessions.",
    },
    {
      type: "local_docker",
      label: "Docker",
      description: "Runs Docker containers on this machine.",
    },
    {
      type: "remote_docker",
      label: "Remote Docker",
      description: "Connects to a remote Docker host.",
    },
    {
      type: "sprites",
      label: SPRITES_DEV,
      description: "Runs agents in Sprites.dev cloud sandboxes.",
    },
    {
      type: "ssh",
      label: SSH,
      description: "Connects to a remote host over SSH and runs agentctl there.",
    },
    {
      type: "k8s",
      label: KUBERNETES,
      description: "Runs agents in administrator-configured Kubernetes Pods.",
    },
  ];

  it.each(CREATE_COPY)("renders $type unchanged", ({ type, label, description }) => {
    const info = EXECUTOR_TYPE_MAP[type];
    expect(executorTypeLabel(info, t)).toBe(label);
    expect(t(info.descriptionKey)).toBe(description);
  });

  it("keeps the persisted executor enum and backend ids out of the catalog", () => {
    expect(Object.keys(EXECUTOR_TYPE_MAP).sort()).toEqual([
      "k8s",
      "local",
      "local_docker",
      "remote_docker",
      "sprites",
      "ssh",
      "worktree",
    ]);
    expect(EXECUTOR_TYPE_MAP.ssh.executorId).toBe("exec-ssh");
    expect(EXECUTOR_TYPE_MAP.k8s.executorId).toBe("exec-k8s");
    expect(EXECUTOR_TYPE_MAP.local_docker.executorId).toBe("exec-local-docker");
  });
});

describe("getExecutorDescription", () => {
  it.each([
    // `local` is the SEEDED Executor.type and reached this helper unmapped
    // before #2249's follow-up, so a local profile's header read "Custom
    // executor." Both spellings must resolve to the same sentence.
    ["local", "Runs agents directly in the repository folder."],
    ["local_pc", "Runs agents directly in the repository folder."],
    ["worktree", "Creates git worktrees for isolated agent sessions."],
    ["local_docker", "Runs Docker containers on this machine."],
    ["remote_docker", "Connects to a remote Docker host."],
    ["sprites", "Runs agents in Sprites.dev cloud sandboxes."],
    ["ssh", "Runs agents on a trusted Linux amd64 or macOS host over SSH."],
    ["k8s", "Runs agents in administrator-configured Kubernetes Pods."],
  ] as const)("renders %s unchanged", (type, expected) => {
    expect(getExecutorDescription(type)).toBe(expected);
  });

  it("falls back to the custom-executor sentence", () => {
    expect(getExecutorDescription("something-else" as never)).toBe("Custom executor.");
  });

  it("differs from the create page on SSH, which is why they do not share a key", () => {
    expect(getExecutorDescription("ssh")).not.toBe(t(EXECUTOR_TYPE_MAP.ssh.descriptionKey));
  });
});

describe("getExecutorLabel", () => {
  it("returns brand and protocol names verbatim rather than from the catalog", () => {
    expect(getExecutorLabel("sprites")).toBe(SPRITES_DEV);
    expect(getExecutorLabel("ssh")).toBe(SSH);
  });

  it("renders the translated labels unchanged", () => {
    expect(getExecutorLabel("local")).toBe("Local");
    expect(getExecutorLabel("worktree")).toBe("Worktree");
    expect(getExecutorLabel("local_docker")).toBe("Local Docker");
    expect(getExecutorLabel("remote_docker")).toBe("Remote Docker");
    expect(getExecutorLabel("k8s")).toBe(KUBERNETES);
  });

  it("echoes an unmapped wire value", () => {
    expect(getExecutorLabel("does-not-exist")).toBe("does-not-exist");
  });

  // The label used to be a module-scope constant. It resolves from the catalog
  // now, so this pins that it resolves at CALL time rather than at import — the
  // property every consumer's re-render depends on.
  it("follows a locale switch", async () => {
    const before = getExecutorLabel("local");
    await activateLocale("pseudo");
    expect(getExecutorLabel("local")).not.toBe(before);
    expect(getExecutorLabel("local")).toBe(t("executors:typeLocal"));
    // Brand names are not catalog values, so they are unaffected.
    expect(getExecutorLabel("ssh")).toBe(SSH);
    await activateLocale("en");
    expect(getExecutorLabel("local")).toBe(before);
  });

  afterAll(async () => {
    if (i18n.language !== "en") await activateLocale("en");
  });
});

describe("validateMcpPolicy", () => {
  it("returns null for empty and valid policies", () => {
    expect(validateMcpPolicy(undefined)).toBeNull();
    expect(validateMcpPolicy("   ")).toBeNull();
    expect(validateMcpPolicy('{"allow_stdio":true}')).toBeNull();
  });

  it("returns a catalog key rather than English for unparseable JSON", () => {
    const key = validateMcpPolicy("{oops");
    expect(key).toBe("executors:invalidJson");
    expect(t(key as string)).toBe("Invalid JSON");
  });

  it("returns a catalog key rather than English for a non-object policy", () => {
    for (const raw of ["[1,2]", '"nope"', "42"]) {
      const key = validateMcpPolicy(raw);
      expect(key).toBe("executors:mcpPolicyMustBeAJsonObject");
      expect(t(key as string)).toBe("MCP policy must be a JSON object");
    }
  });
});
