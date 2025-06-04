{
  description = "A very basic flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      system = "x86_64-linux";

      pkgs = import nixpkgs { inherit system; };
    in
    {
      devShells.${system}.default =
        with pkgs;
        pkgs.mkShell {
          buildInputs = [
            go
          ];
        };

      packages.${system}.default = pkgs.buildGoModule {
        src = self;
        name = "maestro";
        vendorHash = "sha256-rELkSYwqfMFX++w6e7/7suzPaB91GhbqFsLaYCeeIm4=";
      };
    };
}
