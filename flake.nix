{
  description = "dots: a brief, declarative, flexible dotfiles manager";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        dots = pkgs.buildGoModule (finalAttrs: {
          pname = "dots";
          # Untagged checkouts have no release version: use the last release
          # line plus the git revision (dirty trees get "<rev>-dirty") so the
          # string is reproducible and traceable. Update on each release.
          version = "0.2.0-${self.shortRev or self.dirtyShortRev or "dirty"}";
          src = self;

          vendorHash = "sha256-2gzkLRoRsnWueOiFUR5+QlnKxU9ttJyxuk1TW/nsp9A=";

          subPackages = [ "cmd/dots" ];
          env.CGO_ENABLED = "0";
          ldflags = [
            "-s"
            "-w"
            "-X main.version=${finalAttrs.version}"
          ];

          # Tests use an in-memory filesystem, so the default checkPhase works
          # inside the Nix sandbox.

          meta = {
            description = "A brief, declarative, flexible dotfiles manager";
            homepage = "https://github.com/shiroppi/dots";
            license = pkgs.lib.licenses.mit;
            mainProgram = "dots";
            platforms = pkgs.lib.platforms.unix;
          };
        });
        default = dots;
      });
    };
}
