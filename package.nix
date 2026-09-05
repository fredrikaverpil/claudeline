# Buildable against any nixpkgs, independently of flake.nix:
#   pkgs.callPackage ./package.nix { }
# flake.nix passes `rev` so the binary can report the commit it was built from.
{
  lib,
  buildGoModule,
  rev ? "",
}:

buildGoModule rec {
  pname = "claudeline";
  version = "0.24.2"; # x-release-please-version
  src = ./.;
  vendorHash = null; # stdlib only, so there is no go.sum to vendor
  subPackages = [ "." ]; # .pocket/ is a separate module
  # Mirrors .goreleaser.yml so Nix and release builds report the same thing.
  # An empty commit is omitted by main.buildVersion.
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X main.commit=${rev}"
  ];
  meta = {
    description = "A minimalistic and opinionated Claude Code status line";
    homepage = "https://github.com/fredrikaverpil/claudeline";
    license = lib.licenses.mit;
    mainProgram = "claudeline";
  };
}
