{ pkgs }:

pkgs.buildGoModule {
  pname = "engram-backend";
  version = "0.1.0";

  src = ../backend;

  vendorHash = "sha256-TegjL3FtAg2kOxRgxzO1PWHZQWd6gUwMEjk/sFKqA/o=";
  subPackages = [ "." ];

  meta = {
    description = "Engram read-only metadata API";
    mainProgram = "engram";
  };
}
