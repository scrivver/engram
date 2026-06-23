{ pkgs }:

pkgs.buildGoModule {
  pname = "engram-backend";
  version = "0.1.0";

  src = ../backend;

  vendorHash = "sha256-wF4jwDIWjNdseimRpeUUrwo0R4Jjm9nLsBl7dfStLlw=";
  subPackages = [ "." ];

  meta = {
    description = "Engram read-only metadata API";
    mainProgram = "engram";
  };
}
