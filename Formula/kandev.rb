class Kandev < Formula
  desc "Manage tasks, orchestrate agents, review changes, and ship value"
  homepage "https://github.com/kdlbs/kandev"
  url "https://github.com/kdlbs/kandev/archive/refs/tags/v0.41.0.tar.gz"
  sha256 "dbb689fc3dd0ca7fed00f33da8b10ecfec9f9cd2e49bd9908c4950026743073c"
  license "AGPL-3.0-only"

  livecheck do
    url :stable
    strategy :github_latest
  end

  depends_on "go"   => :build
  depends_on "pnpm" => :build
  depends_on "node"

  uses_from_macos "sqlite"

  def install
    ENV["KANDEV_VERSION"] = version.to_s
    ENV["CGO_ENABLED"]    = "1"

    system "pnpm", "-C", "apps", "install", "--frozen-lockfile"
    system "pnpm", "-C", "apps", "--filter", "@kandev/web", "build"
    system "bash", "scripts/release/package-web.sh"
    system "bash", "scripts/release/package-cli.sh"

    bundle = buildpath/"dist/kandev"
    (bundle/"bin").mkpath
    (bundle/"web").mkpath

    cd "apps/backend" do
      system "go", "build",
             "-ldflags", "-s -w -X main.Version=#{version}",
             "-o", bundle/"bin/kandev",
             "./cmd/kandev"
      system "go", "build",
             "-ldflags", "-s -w",
             "-o", bundle/"bin/agentctl",
             "./cmd/agentctl"
    end

    cp_r Dir[buildpath/"dist/web/*"], bundle/"web"

    chmod 0755, bundle/"cli/bin/cli.js"
    libexec.install Dir[bundle/"*"]

    (bin/"kandev").write_env_script libexec/"cli/bin/cli.js",
      KANDEV_BUNDLE_DIR: libexec.to_s,
      KANDEV_VERSION:    version.to_s
  end

  test do
    assert_match "kandev launcher", shell_output("#{bin}/kandev --help")
    assert_match(/\d+\.\d+\.\d+/, shell_output("#{bin}/kandev --version"))
  end
end
