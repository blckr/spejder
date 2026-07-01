{
  description = "spejder — eBPF network monitor";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, ... }@inputs:

    let
      goVersion = 26;

      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forEachSupportedSystem =
        f:
        inputs.nixpkgs.lib.genAttrs supportedSystems (
          system:
          f {
            pkgs = import inputs.nixpkgs {
              inherit system;
              overlays = [ inputs.self.overlays.default ];
            };
          }
        );
    in
    {
      overlays.default = final: prev: {
        go = final."go_1_${toString goVersion}";
      };

      packages = forEachSupportedSystem (
        { pkgs }:
        {
          default = pkgs.callPackage ./nix/package.nix { };
        }
      );

      nixosModules.default = import ./nix/module.nix;

      devShells = forEachSupportedSystem (
        { pkgs }:
        {
          default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              # go (version is specified by overlay)
              go

              gotools
              golangci-lint
              gopls
              delve
              golangci-lint-langserver

              gcc

              # eBPF toolchain
              clang
              llvm
              libbpf
              linuxHeaders
              clang-tools # clangd für den LSP

              # task runner
              just

              # sync with server
              mutagen

              # download MaxMind GeoLite2 databases
              geoipupdate
            ];

            shellHook = ''
                            export BPF_CLANG="${pkgs.clang.cc}/bin/clang"
                            export LIBBPF_INCLUDE="${pkgs.libbpf}/include"
                            export LINUX_INCLUDE="${pkgs.linuxHeaders}/include"

                            cat > .clangd <<EOF
              CompileFlags:
                Add:
                  - -target
                  - bpf
                  - -I${pkgs.libbpf}/include
                  - -I${pkgs.linuxHeaders}/include
                  - -D__BPF_TRACING__
                  - -D__KERNEL__
              EOF
            '';
          };
        }
      );
    };
}
