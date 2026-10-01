# Registered Agent Runtime Coverage

Base inventory: `Registry.LoadDefaults` on main `517249b5e609`, 2026-10-01. Agents owns these runtime capabilities. This is a delivery evidence matrix; the living contract is the [owning design](../../specs/agents/system-design/runtime-update-notifications.md).

`Managed` means exact package staging, protocol candidate validation, selection activation and older-version/default recovery. `Manual` means read-only release discovery when verifiable and vendor/package guidance; Kandev does not advertise safe automatic activation. Current version is a successful host capability observation or the exact managed selection. Non-SemVer, absent, or mismatched observations remain unknown. Latest failure stays unknown. Disabled/unavailable entries do not contact sources or update.

| Registered identity | Runtime/source and owner | Current/latest discovery | Update/activation | Rollback and dependencies |
| --- | --- | --- | --- | --- |
| claude-acp | @agentclientprotocol/claude-agent-acp; Kandev managed | Exact effective package / npm stable latest | Managed; opt-in automatic | Published stable selection/default. Native Claude login/passthrough CLI is separate and not updated. |
| codex-acp | @agentclientprotocol/codex-acp; Kandev managed | Exact effective package / npm stable latest | Managed; opt-in automatic | Published stable selection/default. Native Codex login/passthrough CLI is separate. |
| codex-app-server | @openai/codex; Kandev managed native protocol package | Exact effective package / npm stable latest | Managed app-server candidate; opt-in automatic when enabled | Published stable selection/default; no mutation of global codex. |
| copilot-acp | @github/copilot; Kandev managed | Exact effective package / npm stable latest | Managed; opt-in automatic | Published stable selection/default; native passthrough installation remains external. |
| gemini | @google/gemini-cli; Kandev managed | Exact effective package / npm stable latest | Managed; opt-in automatic | Published stable selection/default. |
| opencode-acp (managed fallback) | opencode-ai; Kandev managed | Exact effective package / npm stable latest | Managed; opt-in automatic only when the host uses managed runtime | Published stable selection/default. |
| opencode-acp (native PATH) | opencode; external native installation | Host protocol version / npm stable package release | Manual vendor upgrade; Kandev cannot infer npm/brew/curl ownership | Vendor supports explicit upgrade target/method; Kandev cannot validate before in-place activation. No auto-update claim. |
| pi-acp | pi-acp; Kandev managed | Exact effective package / npm stable latest | Managed; opt-in automatic | Published stable selection/default. @earendil-works/pi-coding-agent CLI is a separate dependency. |
| muse-acp | @bex-co/muse-code-acp; Kandev managed | Exact effective package / npm stable latest | Managed adapter; opt-in automatic | Published stable selection/default. Native muse installed by Meta is separate/manual. |
| auggie | @augmentcode/auggie; unmanaged npm execution | Host observed version / npm stable latest | Manual package/vendor guidance; launch is unpinned | No managed selection or validated rollback contract. |
| amp-acp | amp-acp; unmanaged npm adapter | Host observed version (unknown if not comparable) / npm stable latest | Manual package guidance | Native Amp is separate; no managed rollback. |
| qwen-acp | @qwen-code/qwen-code; unmanaged npm execution | Host observed version / npm stable latest | Manual package guidance | No managed rollback. |
| iflow-acp | @iflow-ai/iflow-cli; unmanaged npm execution | Host observed version / npm stable latest | Manual package guidance | Experimental ACP; no managed rollback. |
| droid-acp | droid; unmanaged npm execution | Host observed version / npm stable latest | Manual package guidance | No managed rollback. |
| kilocode-acp | @kilocode/cli; unmanaged npm execution | Host observed version / npm stable latest | Manual package guidance | No managed rollback. |
| cursor-acp | cursor-agent; external vendor binary | Host observation; latest unknown (opaque vendor channel) | Verified vendor update guidance, manual | Vendor auto-update is vendor-owned; no isolated validation/rollback contract established. |
| kimi-acp | kimi; external native binary | Host observation / verified GitHub stable release | Manual vendor guidance | No Kandev-owned isolated activation/rollback. |
| minimax-acp | mcode, @minimax-ai/code; external native installation | Host observation / npm stable latest | Manual vendor/package guidance | Installer requires scripts/optional dependencies; no isolated activation/rollback. |
| kiro-acp | kiro-cli-chat; external vendor binary | Host observation; latest unknown | Manual vendor guidance | No verified safe Kandev updater/rollback. |
| qoder-acp | qodercli; external vendor binary | Host observation; latest unknown | Manual vendor guidance | No verified safe Kandev updater/rollback. |
| trae-acp | traecli; external vendor binary | Host observation; latest unknown | Manual vendor guidance | No verified safe Kandev updater/rollback. |
| omp-acp | omp, @oh-my-pi/pi-coding-agent; external bun installation | Host observation / npm stable latest | Manual package guidance | Runtime package ownership remains external; no validated rollback. |
| devin-acp | devin; external vendor binary | Host observation; latest unknown | Manual vendor guidance | No verified safe Kandev updater/rollback. |
| grok-acp | grok, @xai-official/grok; external native installation | Host observation / npm stable latest | Manual package/vendor guidance | Built-in command uses --no-auto-update; no Kandev isolated activation. |
| hermes-acp | hermes; external vendor installation | Host observation / verified GitHub stable release | Verified vendor update guidance, manual | Vendor update is installation-aware (git/Docker/Nix); Kandev cannot assume its owner or restore it. |
| goose-acp | goose; external native installation | Host observation / verified GitHub stable release | Manual vendor guidance | Install path chosen externally; no isolated Kandev activation/rollback. |
| antigravity-acp | configurable ACP executable; externally managed | Host observation; latest unknown | Manual vendor guidance | No installer/updater supplied by agent metadata. |
| dynamic | virtual router, no independent executable | Not applicable | Unsupported; concrete routed agent owns runtime | No separate runtime to update. |
| mock-agent | test fixture, Kandev binary | Test/developer fixture version; no production source | Unsupported | Built with Kandev; never install a guessed package. |
| Custom ACP and TUI registrations | Operator command; externally managed/custom | Host observation where available; latest unknown | Unsupported automatic; operator guidance | Arbitrary commands never authorize package/update execution or rollback. |

## Native mechanism evidence

- [OpenCode CLI](https://opencode.ai/docs/cli/) documents `opencode upgrade` with an explicit version and installation method. PATH alone cannot choose that method or provide candidate isolation. PR #4014 proposes a different guarded selection boundary; it is not main behavior or a dependency here.
- [Cursor installation](https://docs.cursor.com/en/cli/installation) documents `cursor-agent update`/`upgrade` and vendor-owned automatic updates. No verified isolated candidate or rollback is established.
- [Hermes installation](https://hermes-agent.nousresearch.com/docs/getting-started/installation) documents installation-aware `hermes update` behavior; it can refer the operator to Docker/Nix workflows. Kandev does not guess an updater from the binary name.
- Native Goose/Kimi release repository metadata is read-only; stable releases must reject draft/prerelease and non-SemVer versions. Failure means unknown, not current.
- [Kimi legacy installation and migration](https://moonshotai.github.io/kimi-cli/en/guides/getting-started.html) distinguishes legacy kimi-cli releases from the newer Kimi Code CLI. The registered native integration uses the legacy source; this feature does not perform a product migration. [Devin CLI](https://docs.devin.ai/cli) provides verified vendor guidance; no isolated activation/rollback contract is assumed.
- Existing managed native protocol validation is exercised through Codex app-server, in addition to multiple ACP identities. No developer global CLI or auth state is changed in deterministic tests.

## Validation evidence

The registry coverage test enumerates all 28 default registrations and asserts their managed/manual/unsupported modes, so new registrations require a deliberate coverage decision. Owning-package race suites and the live-process/rollback/default tests passed. Desktop and phone browser evidence and source-failure/deduplication checks are recorded in work orders 01–03. External native installations have no automatic activation claim.
