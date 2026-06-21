# Template used by update-homebrew-tap.sh. Placeholder strings are replaced at
# release time before this formula is pushed to kdlbs/homebrew-kandev.
class Kandev < Formula
  desc "Manage tasks, orchestrate agents, review changes, and ship value"
  homepage "https://github.com/kdlbs/kandev"
  license "AGPL-3.0-only"
  version "__VERSION__"

  on_macos do
    if Hardware::CPU.arm?
      url "__GITHUB_BASE__/kandev-macos-arm64.tar.gz"
      sha256 "__SHA_MACOS_ARM64__"
    else
      url "__GITHUB_BASE__/kandev-macos-x64.tar.gz"
      sha256 "__SHA_MACOS_X64__"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "__GITHUB_BASE__/kandev-linux-arm64.tar.gz"
      sha256 "__SHA_LINUX_ARM64__"
    else
      url "__GITHUB_BASE__/kandev-linux-x64.tar.gz"
      sha256 "__SHA_LINUX_X64__"
    end
  end

  def install
    libexec.install Dir["*"]
    # Create a stable wrapper at $HOMEBREW_PREFIX/bin/kandev that points at the
    # native launcher in the Cellar and sets the bundle/version env it uses to
    # find bin/.
    (bin/"kandev").write_env_script libexec/"bin/kandev",
      KANDEV_BUNDLE_DIR: libexec.to_s,
      KANDEV_VERSION:    version.to_s
  end

  test do
    assert_match "kandev launcher", shell_output("#{bin}/kandev --help")
  end
end
