---
status: current
system: agents
requirements:
  - REQ-AGENTS-PI-REMOTE-AUTH-001
  - REQ-AGENTS-PI-REMOTE-AUTH-002
---

# Pi remote credentials system design

## Purpose and boundaries

`agents.PiACP` declares what Pi needs remotely through the existing
`RemoteAuth` and `PortableConfig` capabilities. Catalog building, the
Remote Credentials card, file transfer, JSON merging, secret resolution and
bundle upload are shared code used by every agent; this change adds no new
mechanism.

## Declarations

| Capability | Entry | Target |
|---|---|---|
| `RemoteAuth` files method | `.pi/agent/auth.json`, `merge_json_object` | `.pi/agent` |
| `RemoteAuth` env methods | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `OPENROUTER_API_KEY` | agent environment |
| `PortableConfig` bundle `pi.config` | `.pi/agent/settings.json`, `.pi/agent/models.json` | same paths |

The paths are the same on darwin, linux and windows because Pi resolves its
agent directory as `<home>/.pi/agent`. `PI_CODING_AGENT_DIR`, which moves that
directory, is not read: the host side and the remote side both use the
default.

The files method sets no label, so the card renders its translated
`executors:copyAuthFiles`. The bundle label is mapped by ID in
`portable-config-bundles.tsx` to `executors:portableConfigBundlePiConfig`.

## Credential precedence

Pi uses a stored `auth.json` credential before a provider's environment
variable, so a copied auth file wins over a selected secret for the same
provider. `models.json` can hold inline `apiKey` values; the bundle copies the
file as it is.

## SSH forwarding

The SSH executor forwards only `sshRemoteAgentCredentialEnvKeys` from the
resolved request environment, so a Pi env method works over SSH only when its
variable is in that list. `OPENROUTER_API_KEY` is added; the other three were
already present.

## Verification

`pi_acp_test.go` pins the declarations, the merge policy, the missing label
and the bundle's exclusion of `auth.json`.
`executor_ssh_pi_env_test.go` derives Pi's env variables from its declaration
and checks that SSH forwards each one.
