#!/usr/bin/env bash
# Build the CLI and copy artifacts into dist/kandev/cli/ for inclusion in release bundles.
#
# The CLI bundle (dist/cli.bundle.js) is a single-file esbuild output with all Node
# dependencies inlined. This is required for Homebrew installs where there is no
# node_modules directory in the Cellar.
#
# Output layout inside dist/kandev/cli/:
#   bin/cli.js          - Node entrypoint (requires #!/usr/bin/env node shebang + chmod +x)
#   dist/cli.bundle.js  - Self-contained bundle with inlined deps (used by Homebrew)
#   package.json        - Package metadata (used to read version at runtime)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLI_DIR="$ROOT_DIR/apps/cli"
OUT_DIR="$ROOT_DIR/dist/kandev/cli"

echo "Building CLI for release bundle..."

# Build TypeScript
pnpm -C "$ROOT_DIR/apps" --filter kandev build
echo "  TypeScript build complete"

# Bundle with esbuild (inlines tree-kill and all other deps)
pnpm -C "$ROOT_DIR/apps/cli" bundle
echo "  esbuild bundle complete"

# Copy artifacts into dist/kandev/cli/
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/bin" "$OUT_DIR/dist"

# Write a bin/cli.js that points at the bundled dist (for Homebrew/manual installs
# where there is no node_modules to resolve the unbundled dist/cli.js's deps).
# The npm package's bin/cli.js is different — it requires the unbundled dist/cli.js
# because tree-kill etc. are installed normally via npm.
cat > "$OUT_DIR/bin/cli.js" <<'EOF'
#!/usr/bin/env node

require("../dist/cli.bundle.js");
EOF
chmod +x "$OUT_DIR/bin/cli.js"

cp "$CLI_DIR/dist/cli.bundle.js" "$OUT_DIR/dist/cli.bundle.js"
cp "$CLI_DIR/package.json" "$OUT_DIR/package.json"

echo "CLI artifacts packaged at $OUT_DIR"
echo "  bin/cli.js (entrypoint, requires dist/cli.bundle.js, chmod +x)"
echo "  dist/cli.bundle.js (self-contained bundle, deps inlined)"
echo "  package.json"
