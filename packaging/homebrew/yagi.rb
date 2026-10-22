# The Homebrew formula for Yagi, which the release workflow fills in (scripts/render_formula.sh) and pushes to the
# tap, johnschult/homebrew-yagi, on every release. The @…@ marks are replaced with the release's version and the
# SHA-256 of each of its archives. It installs the program the release already built; nothing is built by Homebrew.
class Yagi < Formula
  desc "Share an HDHomeRun tuner with the Plex, Jellyfin and Emby of family and friends"
  homepage "https://github.com/johnschult/yagi"
  version "@VERSION@"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/johnschult/yagi/releases/download/v#{version}/yagi_#{version}_darwin_arm64.tar.gz"
      sha256 "@SHA_DARWIN_ARM64@"
    end
    on_intel do
      url "https://github.com/johnschult/yagi/releases/download/v#{version}/yagi_#{version}_darwin_amd64.tar.gz"
      sha256 "@SHA_DARWIN_AMD64@"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/johnschult/yagi/releases/download/v#{version}/yagi_#{version}_linux_arm64.tar.gz"
      sha256 "@SHA_LINUX_ARM64@"
    end
    on_intel do
      url "https://github.com/johnschult/yagi/releases/download/v#{version}/yagi_#{version}_linux_amd64.tar.gz"
      sha256 "@SHA_LINUX_AMD64@"
    end
  end

  def install
    bin.install "yagi"
  end

  def post_install
    (var/"yagi").mkpath
  end

  # The settings live in var/yagi, not in whatever folder it was started from.
  service do
    run [opt_bin/"yagi", "-config", var/"yagi/yagi.json"]
    keep_alive true
    working_dir var/"yagi"
    log_path var/"log/yagi.log"
    error_log_path var/"log/yagi.log"
  end

  def caveats
    <<~EOS
      To run Yagi in the background, and again after a restart:
        brew services start yagi

      Its settings file is #{var}/yagi/yagi.json, and its log is #{var}/log/yagi.log. The log has the one-time
      setup link that makes you the admin; the web page is at http://localhost:9244/ on this machine.
      To try it in a terminal first, run: yagi
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/yagi -version")
  end
end
