# Follow-up proposal: a generic formatter seam and an independent plugin

Thank you for the earlier feedback on #5011. The fork implementation linked
above has been useful operationally. We would now like to separate its product
logic from Kandev so template changes do not require a custom host build.

Would you be open to two coordinated contributions?

1. **Kandev PR:** a minimal, opt-in, provider-bound notification formatter
   extension in the public plugin contract. Kandev retains recipient ownership,
   enabled/subscription checks, occurrence claims, runtime-batch claims,
   transport credentials, sending, and transport failure handling.
2. **Independent plugin PR:** the literal template renderer, revisioned template
   storage, variable catalog, synthetic preview, editor, and legacy import tool.
   The plugin lives outside this monorepo and does not send notifications itself.

## Why a host extension?

On the inspected `v0.99.0` source, `Plugin.DeliverEvent`/`OnEvent` observes bus
events rather than the admitted notification delivery. Reimplementing delivery
from those events would duplicate recipient/subscription/deduplication policy.
The Webhook provider preserves that policy but its version 1.0 payload contains
only the default copy and task/session/occurrence IDs, not the complete typed
inbox and runtime-update context used by the existing template variables.

The proposal is a host-to-plugin formatter call after delivery admission and
before the existing transport. It is not a second event subscriber or sender.
Each provider explicitly selects at most one formatter implementation. The host
would supply a bounded, versioned semantic DTO with default title/body and only
authorized event context. It would never supply provider config, receiver URLs,
credentials, transcripts, question text, or arbitrary internal payload maps.

The response can replace only title/body or request the original defaults.
Unsupported contracts, unavailable plugins, deadline expiry, malformed output,
and invalid UTF-8/title/output bounds fall back to the whole original pair.
Formatting never changes recipients, subscription state, occurrence identity,
transport, or runtime-summary membership, and never causes an extra send.

## Configuration and UI boundary

Core would own only the formatter declaration, provider binding, lifecycle and
owner-authorized management relay/native UI mount. The plugin would own its
template schema, parser, per-provider/event revisions, synthetic examples, and
editor. Management calls must be bound by core to the authenticated provider
owner and selected plugin installation; neither browser-supplied IDs nor public
webhooks would establish that authority. This may need a provider resource scope
in the generic authenticated action contract. Exact public names and DTOs remain
proposals for discussion, not existing SDK methods.

The first plugin would format Apprise only. Other built-in providers retain
their current content and delivery behavior. There is no implicit formatter
selection when installing a plugin, and the existing provider-test send remains
distinct from non-sending synthetic preview.

## Migration and compatibility

- Export the fork's legacy template records from a private, consistent offline
  snapshot using a one-time operator tool. The running plugin never opens
  Kandev's database or calls private APIs.
- Import without changing providers, receiver credentials, subscriptions, or
  history; reject ownership/revision collisions and make identical imports
  repeatable. Preserve the old data until the cutover and rollback drill pass.
- Compare old/new rendering on synthetic fixtures for all four legacy events,
  missing-value cases, update subtypes, and URL encoding before changing a live
  binding. No automatic production upgrade or real recipient test sends.
- Keep existing plugins wire-compatible. A new formatter plugin requires the
  released SDK and actual host baseline that support the extension. An unchanged
  `v0.99.0` is a negative compatibility case, not a supported installation target.
- Develop/review the two contributions against the same pinned contract;
  public plugin release follows host support and SDK publication. Record package
  digests, restart/upgrade/backup/restore results, and untested platforms.

## Alternatives and trade-offs

Keeping templates in core is simpler initially but couples the renderer/editor
to host releases. Sending from `OnEvent` duplicates delivery policy. A webhook
bridge changes transport/configuration and lacks some existing context. The
formatter seam keeps one sender, but adds a bounded RPC and a permission-bearing
configuration boundary that needs explicit review.

This is a change in the proposed architecture. The earlier positive response is
not being treated as acceptance of this new protocol. Following CONTRIBUTING.md,
we are preparing the design package and will wait for maintainer discussion of
this direction before implementation or opening the upstream PR. Would you prefer
this split, or retaining the original Apprise-only core feature?
