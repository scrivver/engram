{ pkgs, infraShell }:

pkgs.mkShell {
  name = "engram-ingestion-shell";
  inputsFrom = [ infraShell ];
  buildInputs = [
    pkgs.python3
    pkgs.uv
    pkgs.ruff
  ];
}
