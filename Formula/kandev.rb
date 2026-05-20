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

    # The Next.js standalone tracer pulls platform-tagged native modules
    # into the bundle, including musl-libc variants of sharp/@swc/lightningcss
    # that brew linkage --test flags on glibc-only Linuxbrew. Strip them.
    Pathname.glob("#{bundle}/web/**/*musl*").each { |p| rm_r(p) if p.directory? }

    libexec.install Dir[bundle/"*"]

    (bin/"kandev").write_env_script libexec/"cli/bin/cli.js",
      KANDEV_BUNDLE_DIR: libexec.to_s,
      KANDEV_VERSION:    version.to_s
  end

  test do
    # Wrapper sanity: confirms write_env_script wired KANDEV_BUNDLE_DIR
    # and the launcher reads the bundled package.json version.
    assert_match version.to_s, shell_output("#{bin}/kandev --version")

    # Functional test: boot the agentctl sidecar (a pure-Go HTTP server
    # bundled alongside the backend binary), poll /health until it
    # responds, then shut down. Exercises Go runtime startup and HTTP
    # listener — the parts most likely to break across platforms — and
    # avoids the larger backend's cgo+sqlite migration runner, which
    # makes the test sandbox-friendly and fast.
    port = free_port
    pid  = spawn(libexec/"bin/agentctl", "-port=#{port}")
    begin
      deadline = Time.now + 60
      # quiet_system (not Formula#system) — the latter raises BuildError
      # on the first non-zero exit, which kills the retry loop before
      # agentctl has had time to bind the port.
      until quiet_system "curl", "-sf", "-o", File::NULL,
                         "http://127.0.0.1:#{port}/health"
        raise "agentctl did not start within 60s" if Time.now > deadline

        sleep 1
      end
      assert_match(/status|ok/i,
                   shell_output("curl -s http://127.0.0.1:#{port}/health"))
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
