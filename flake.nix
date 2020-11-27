{
  description = "sclable-keyserver";
  inputs.nixpkgs = { url = "nixpkgs/nixos-20.09"; };
  outputs = { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
    in rec {
      devShell."${system}" =
        with pkgs;
        mkShell {
          buildInputs = [
            go
          ];
          shellHook = ''
            go version
          '';
        };

      defaultPackage."${system}" = pkgs.stdenvNoCC.mkDerivation rec {
              name = "lac-thesis";
              buildInputs = [
                pkgs.go
              ];
              src = pkgs.lib.cleanSource ./.;
              /* src = ./.; */
              buildPhase = ''
                go build
                ls -al
              '';
              installPhase = ''
                touch $out
              '';
            };
  };
}
