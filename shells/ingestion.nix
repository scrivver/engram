{ pkgs, infraShell }:

pkgs.mkShell {
  name = "engram-ingestion-shell";
  inputsFrom = [ infraShell ];
  buildInputs = [
    pkgs.python3
    pkgs.uv
    pkgs.ruff
    pkgs.file  # provides libmagic for python-magic
  ];

  shellHook = ''
    export LD_LIBRARY_PATH="${pkgs.file}/lib''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
  '';
}
