{ pkgs, infraShell }:

pkgs.mkShell {
  name = "engram-backend-shell";
  inputsFrom = [ infraShell ];
  buildInputs = [
    pkgs.go
    pkgs.gopls
    pkgs.gotools
    pkgs.air
  ];
}
