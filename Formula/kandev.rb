class Kandev < Formula
  desc "Manage tasks, orchestrate agents, review changes, and ship value"
  homepage "https://github.com/kdlbs/kandev"
  url "https://github.com/kdlbs/kandev/archive/refs/tags/v0.50.0.tar.gz"
  sha256 "270d4c17f0cdd8b431e050f130dfee7d0f962149e3e0f50e149efec21a95954c"
  license "AGPL-3.0-only"

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)+)$/i)
  end

  depends_on "go"   => :build
  depends_on "pnpm" => :build

  uses_from_macos "sqlite"

  def install
    ENV["KANDEV_VERSION"] = version.to_s

    system "pnpm", "-C", "apps", "install", "--frozen-lockfile"
    system "pnpm", "-C", "apps", "--filter", "@kandev/web", "build"
    system "make", "sync-embedded-web"

    bundle = buildpath/"dist/kandev"
    (bundle/"bin").mkpath

    cd "apps/backend" do
      # kandev backend needs cgo for mattn/go-sqlite3.
      with_env(CGO_ENABLED: "1") do
        system "go", "build",
               "-tags", "fts5",
               *std_go_args(ldflags: "-s -w -X main.Version=#{version}",
                            output:  bundle/"bin/kandev"),
               "./cmd/kandev"
      end
      # agentctl is pure-Go; build it static to avoid a dynamic linker
      # crash observed on Linuxbrew arm64 bottle CI.
      with_env(CGO_ENABLED: "0") do
        system "go", "build",
               *std_go_args(ldflags: "-s -w", output: bundle/"bin/agentctl"),
               "./cmd/agentctl"
      end
    end

    system "./scripts/release/package-bundle.sh"

    libexec.install Dir[bundle/"*"]

    (bin/"kandev").write_env_script libexec/"bin/kandev",
      KANDEV_BUNDLE_DIR: libexec.to_s,
      KANDEV_VERSION:    version.to_s
  end

  test do
    # Wrapper sanity: confirms write_env_script wired KANDEV_BUNDLE_DIR
    # and the launcher reads the bundled package.json version.
    assert_match version.to_s, shell_output("#{bin}/kandev --version")

    # Functional test: exercise the agentctl sidecar's CLI subcommand
    # dispatcher. Boots the Go binary, parses flags, dispatches to the
    # kandev subcommand handler, and prints usage when called without
    # arguments. Verifies the binary loads correctly across platforms
    # without requiring HTTP listeners, port binds, or subprocesses —
    # which can be problematic in sandboxed build environments.
    output = shell_output("#{libexec}/bin/agentctl kandev 2>&1", 1)
    assert_match "Usage: agentctl kandev", output
  end
end
