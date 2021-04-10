class Pcp < Formula
  desc "📦 Command-line peer-to-peer data transfer tool based on libp2p"
  homepage "https://github.com/dennis-tra/pcp"
  url "https://github.com/dennis-tra/pcp/archive/refs/tags/v0.4.0.tar.gz"
  sha256 "09bf477afcca5aabd617c90f012063786715ab5715cce77a72f2a0ae758585ea"
  license "Apache-2.0"
  head "https://github.com/dennis-tra/pcp.git"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(:ldflags => "-X main.RawVersion=0.4.0 -X main.ShortCommit=7f638fe"), "cmd/pcp/pcp.go"
  end

  test do
    assert_match shell_output("#{bin}/pcp receive words-that-dont-exist", 1).chomp, "error: failed to initialize node: could not find all words in a single wordlist"
  end
end
