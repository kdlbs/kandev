// Command plugin-fixture is an SDK-based kandev plugin backend used by Go
// integration tests and Playwright e2e tests: it proves the gRPC plugin
// contract (docs/plans/plugins/GRPC-CONTRACT.md) end to end without
// depending on internal/plugins, since it only imports the public
// pkg/pluginsdk surface — exactly what a third-party plugin author would.
//
// Its companion package directory, fixture-package/, holds the manifest and
// UI bundle a test packages this binary with (see
// docs/plans/plugins/GRPC-CONTRACT.md §6 and `make e2e-plugin-package` in
// apps/backend/Makefile).
package main

import "github.com/kandev/kandev/pkg/pluginsdk"

func main() {
	pluginsdk.Serve(newFixturePlugin())
}
