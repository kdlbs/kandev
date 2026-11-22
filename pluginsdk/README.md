# Kandev Go plugin SDK

Module: `github.com/kdlbs/kandev/pluginsdk`. The shared Go/npm SDK version
starts at `0.1.0` and is independent of the Kandev host version. Public
availability requires a successful Stable SDK release; an untagged checkout
with null host metadata is not a published SDK.

After public verification:

```bash
go get github.com/kdlbs/kandev/pluginsdk@v0.1.0
```

```go
import "github.com/kdlbs/kandev/pluginsdk"

func main() { pluginsdk.Serve(&pluginsdk.UnimplementedPlugin{}) }
```

Read `sdk-release.json` for the originating host, whole-SDK host floor, and
supported manifest API versions. Run the checker in the consumer project:

```bash
go run github.com/kdlbs/kandev/pluginsdk/cmd/kandev-sdk-check@v0.1.0 --manifest manifest.yaml
```

For a frontend dependency, add `--frontend-sdk node_modules/@kdlbs/plugin-sdk`.
The checker reads installed versions and rejects local SDK substitutions. It
does not edit project files. Keep exact dependency pins and commit lockfiles.

See the [public authoring guide](https://github.com/kdlbs/kandev/blob/main/docs/public/plugins-authoring.md)
for migration, compatibility, permissions, and packaging. The source move
preserves the existing handshake and wire contract. Old source imports need
migration; already built binaries retain their existing runtime contract.

Contributors run `go test -race ./...`, `go vet ./...`, and `make proto` from
this module. The host's local replacement is a development dependency, not a
plugin release pattern. The SDK is licensed under AGPL-3.0-only; see `LICENSE`.
