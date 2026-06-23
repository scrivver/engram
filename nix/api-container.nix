{ pkgs }:

let
  backend = import ./backend.nix { inherit pkgs; };
  healthcheck = pkgs.writeShellScriptBin "engram-api-healthcheck" ''
    exec ${pkgs.curl}/bin/curl --fail --silent --show-error \
      http://127.0.0.1:8081/api/health >/dev/null
  '';
in
pkgs.dockerTools.buildLayeredImage {
  name = "engram-api";
  tag = "latest";

  contents = [
    backend
    healthcheck
    pkgs.cacert
    pkgs.curl
  ];

  config = {
    Entrypoint = [ "${backend}/bin/engram" ];
    ExposedPorts = { "8081/tcp" = {}; };
    Env = [
      "PORT=8081"
      "PGHOST=postgres"
      "PGUSER=postgres"
      "PGDATABASE=engram"
      "OIDC_USERNAME_CLAIM=preferred_username"
      "SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
    ];
  };
}
