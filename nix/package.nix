{ lib, buildGoModule }:

buildGoModule {
  pname = "spejder";
  version = "0.1.0";
  src = ../.;

  subPackages = [
    "cmd/spejder-daemon"
    "cmd/spejder"
  ];

  vendorHash = "sha256-C3gOKH35Zd6N9HO0jtZGmDa2hqEX5+3jBJya5ohmLnY=";
}
