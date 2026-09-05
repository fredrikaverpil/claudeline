{
  description = "A minimalistic and opinionated Claude Code status line";

  # Only a default, so `nix run`/`nix profile install` and CI have something to
  # build against. Consumers should point it at their own nixpkgs:
  #   inputs.claudeline.inputs.nixpkgs.follows = "nixpkgs";
  # or skip this flake and call ./package.nix with their own pkgs.
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-darwin"
        "x86_64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      overlays.default = final: _prev: {
        claudeline = final.callPackage ./package.nix { };
      };

      packages = forAllSystems (pkgs: rec {
        default = claudeline;
        claudeline = pkgs.callPackage ./package.nix { rev = self.rev or ""; };
      });

      # Editor/LSP toolchain. Pocket downloads its own pinned go, golangci-lint,
      # bun and prettier into .pocket/tools, so `./pok` does not need this.
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
          ];
        };
      });
    };
}
