# ADR-2026-10-09-published-plugin-sdk: Independently published plugin SDKs

**Status:** accepted
**Date:** 2026-10-09
**Area:** protocol

## Context

External plugins depend on `github.com/kandev/kandev/pkg/pluginsdk`, but its
backend module path does not match the repository and its subdirectory has no
module-specific release tags. The frontend package is isolated but absent from
the release package inventory. Authors use sibling checkouts and inherit the
backend dependency graph instead of installing an immutable public SDK.

SDK versioning, manifest API policy, and the first host release supporting an
API are separate contracts. Equating their numbers would conceal compatibility
requirements. The user selected independent SDK versions and an SDK-wide host
baseline during planning for issue #4358.

## Decision

Maintain the public Go SDK in a root `pluginsdk/` module named
`github.com/kdlbs/kandev/pluginsdk`. It owns the SDK package and the canonical
plugin protobuf source and generated Go code. The backend consumes this single
definition using a checked-in repository-local replacement; external consumers
install module versions without replacements. Do not rename the backend module
or maintain a second generated protocol definition.

Publish the Go module under `pluginsdk/vX.Y.Z` tags and the npm name
`@kdlbs/plugin-sdk` under the same independent `X.Y.Z`, initially `0.1.0`.
Publish a new shared version for contract changes associated with a Stable host
release. An unchanged host release reuses the previous SDK. SDK patch releases
may address packaging, dependencies, or SDK fixes without changing the host
baseline. Before SDK `1.0.0`, incompatible changes require a minor bump and
explicit migration notes; after `1.0.0`, use normal SemVer, including Go major
import suffixes for `v2` and later.

One canonical SDK release record defines its version, originating host release,
minimum host version, and supported manifest API versions. Published copies are
identical across Go and npm. Release preparation resolves pending host numbers
from the actual Stable release. Author CI validates the actual installed SDKs
and requires a manifest minimum at least as high as every consumed SDK baseline.
Optional features still require detection and declared authority.

The extraction changes source imports with a migration note. Preserve existing
binary transport compatibility. Do not add an alias facade for the old module
path: it would remain unfetchable, add another surface to maintain, and retain
the dependency on the full backend for authors using it.

SDK publication is part of Stable release completion when a new SDK is needed.
Immutable artifact verification precedes reuse; partial publication resumes at
the same version and signed source revision. Nightly does not publish the SDK.
The reconciled design retains the original tested npm archive and an authenticated
receipt. SDK tagging runs after host publication on both normal and recovery
paths. An initial bootstrap state retains the candidate without claiming success.
These recovery details preserve the accepted version and host-baseline choices.

## Namespace amendment (2026-10-26)

The user selected `@kdlbs/plugin-sdk` before the first SDK publication.
The original proposal used `@kandev/plugin-sdk`, matching the local workspace
package. Authenticated npm checks confirmed that account `zval` owns the
`kdlbs` organization but is not a member of the `kandev` organization.
Access to the unscoped `kandev` package does not grant that scope.
Use the controlled organization namespace for publication and source imports.
Do not publish an alias or placeholder under the former name.

This amendment changes only the npm identity. The Go module path, shared
independent version starting at `0.1.0`, SDK-wide host floor, and installed
plugin wire compatibility remain unchanged. Existing frontend source consumers
must update their dependency name, imports, and lockfiles when migrating.
The package remains unpublished until the signed release procedure completes.

## Consequences

External builds and analysis resolve a small public module. Contract changes
require an intentional SDK version and compatibility record update. The host
and SDK remain developed together, while a clean-consumer test proves the local
replacement does not conceal missing package contents or dependency declarations.

Existing Go source imports must migrate. A newer SDK can require a newer host
even when a plugin only uses older APIs. This conservative rule avoids a separate
feature-compatibility registry. Go and npm publication can fail independently;
the release remains partial until both are verified. Maintainers must own the
npm scope and configure its trusted publisher.

## Alternatives considered

- **Use Kandev version numbers for every SDK release:** simple numeric mapping,
  but unrelated host changes would create SDK releases and SDK SemVer could not
  independently describe its source compatibility. The user chose independent
  numbering.
- **Keep a pinned backend checkout or vendor its SDK subset:** reproducible
  snapshots remain possible, but authors still manage repository layout or
  copied source and tools cannot use normal package resolution.
- **Create a dedicated SDK repository:** fetchable, but introduces synchronization
  and release coordination between the host, SDK, and generated protocol.
- **Nest a module under `apps/backend/pkg/pluginsdk`:** reduces movement, but
  exposes the application layout as the permanent public module path and does
  not match the requested concise `github.com/kdlbs/kandev/pluginsdk` identity.
- **Feature-specific host minimums:** allows some newer-SDK plugins on older
  hosts, but requires a second compatibility declaration and validation system.
  The user chose the SDK-wide floor.

## Related artifacts

- [Requirements](../specs/plugins/requirements/sdk-distribution.md).
- [System design](../specs/plugins/system-design/sdk-distribution.md).
- [Implementation plan](../plans/plugin-sdk-publishing/plan.md).
