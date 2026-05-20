class Kandev < Formula
  desc "Manage tasks, orchestrate agents, review changes, and ship value"
  homepage "https://github.com/kdlbs/kandev"
  url "https://github.com/kdlbs/kandev/archive/refs/tags/v0.49.0.tar.gz"
  sha256 "a97f93b7733656d2c128998bce10dd804c7f192bf03112540198aa96e005a449"
  license "AGPL-3.0-only"

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)+)$/i)
  end

  depends_on "go"   => :build
  depends_on "pnpm" => :build
  depends_on "node"

  uses_from_macos "rsync" => :build
  uses_from_macos "sqlite"

  def install
    ENV["KANDEV_VERSION"] = version.to_s
    ENV["CGO_ENABLED"]    = "1"

    system "pnpm", "-C", "apps", "install", "--frozen-lockfile"
    system "pnpm", "-C", "apps", "--filter", "@kandev/web", "build"
    system "./scripts/release/package-web.sh"
    system "./scripts/release/package-cli.sh"

    bundle = buildpath/"dist/kandev"
    (bundle/"bin").mkpath

    cd "apps/backend" do
      system "go", "build",
             *std_go_args(ldflags: "-s -w -X main.Version=#{version}",
                          output:  bundle/"bin/kandev"),
             "./cmd/kandev"
      system "go", "build",
             *std_go_args(ldflags: "-s -w", output: bundle/"bin/agentctl"),
             "./cmd/agentctl"
    end

    system "./scripts/release/package-bundle.sh"

    libexec.install Dir[bundle/"*"]

    (bin/"kandev").write_env_script libexec/"cli/bin/cli.js",
      KANDEV_BUNDLE_DIR: libexec.to_s,
      KANDEV_VERSION:    version.to_s
  end

  test do
    # Wrapper sanity: confirms write_env_script wired KANDEV_BUNDLE_DIR
    # and the launcher reads the bundled package.json version.
    assert_match version.to_s, shell_output("#{bin}/kandev --version")

    # Functional test: boot the Go backend with an isolated data dir,
    # poll /api/v1/system/health until it responds, then shut down.
    # Exercises the cgo+sqlite linkage, migration runner, and HTTP
    # server — the parts most likely to break across platforms.
    port = free_port
    pid = spawn({ "KANDEV_HOME_DIR"    => testpath.to_s,
                  "KANDEV_SERVER_PORT" => port.to_s,
                  "KANDEV_LOG_LEVEL"   => "warn" },
                libexec/"bin/kandev")
    begin
      deadline = Time.now + 60
      until system("curl", "-sf", "-o", File::NULL,
                   "http://127.0.0.1:#{port}/api/v1/system/health")
        raise "kandev backend did not start within 60s" if Time.now > deadline

        sleep 1
      end
      assert_match "healthy",
                   shell_output("curl -s http://127.0.0.1:#{port}/api/v1/system/health")
    ensure
      # Guard against ESRCH if the backend already crashed — without
      # this, an exception in `ensure` masks the original failure
      # diagnostic (e.g. the "did not start within 60s" message).
      begin
        Process.kill("TERM", pid)
        Process.wait(pid)
      rescue Errno::ESRCH, Errno::ECHILD
        nil
      end
    end
  end
end
