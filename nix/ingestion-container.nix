{ pkgs }:

let
  ingestion = import ./ingestion.nix { inherit pkgs; };
  healthcheck = pkgs.writeShellScriptBin "engram-ingestion-healthcheck" ''
    kill -0 1
  '';
in
pkgs.dockerTools.buildLayeredImage {
  name = "engram-ingestion";
  tag = "latest";

  contents = [
    ingestion
    healthcheck
    pkgs.cacert
    pkgs.file
    pkgs.ffmpeg
    pkgs.poppler-utils
    pkgs.tesseract
  ];

  config = {
    Entrypoint = [ "${ingestion}/bin/engram-ingestion" ];
    Env = [
      "PGHOST=postgres"
      "PGUSER=postgres"
      "PGDATABASE=engram"
      "RABBITMQ_HOST=rabbitmq"
      "RABBITMQ_AMQP_PORT=5672"
      "STORAGE_BACKEND=s3"
      "STORAGE_S3_ENDPOINT=http://minio:9000"
      "STORAGE_S3_BUCKET=reliquary"
      "SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
    ];
  };
}
