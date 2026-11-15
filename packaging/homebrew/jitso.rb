# Template. scripts/fill-formula.sh fills the upper-case placeholders written
# between at signs (the version and the four archive checksums) and writes the
# formula for the tap, without this comment.
class Jitso < Formula
  desc "Launch agent sessions from approved builds and log every launch"
  homepage "https://github.com/upex-galaxy/jitso"
  version "@VERSION@"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/upex-galaxy/jitso/releases/download/v@VERSION@/jitso-darwin-arm64.tar.gz"
      sha256 "@SHA256_DARWIN_ARM64@"
    end
    on_intel do
      url "https://github.com/upex-galaxy/jitso/releases/download/v@VERSION@/jitso-darwin-amd64.tar.gz"
      sha256 "@SHA256_DARWIN_AMD64@"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/upex-galaxy/jitso/releases/download/v@VERSION@/jitso-linux-arm64.tar.gz"
      sha256 "@SHA256_LINUX_ARM64@"
    end
    on_intel do
      url "https://github.com/upex-galaxy/jitso/releases/download/v@VERSION@/jitso-linux-amd64.tar.gz"
      sha256 "@SHA256_LINUX_AMD64@"
    end
  end

  def install
    bin.install "jitso"
  end

  test do
    assert_match "jitso v#{version} ", shell_output("#{bin}/jitso version")
  end
end
