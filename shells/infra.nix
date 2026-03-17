{ pkgs, processComposeConfig }:

pkgs.mkShell {
  name = "engram-infra-shell";
  buildInputs = [
    pkgs.rabbitmq-server
    pkgs.postgresql
    pkgs.minio
    pkgs.minio-client
    pkgs.process-compose
    pkgs.python3
    pkgs.curl
    pkgs.nodejs_24
  ];

  shellHook = ''
    export SHELL=${pkgs.bash}/bin/bash
    export PATH="$PWD/bin:$PATH"

    export DATA_DIR="$PWD/.data"
    mkdir -p "$DATA_DIR"
    mkdir -p "$DATA_DIR/rabbitmq"
    mkdir -p "$DATA_DIR/postgres"
    mkdir -p "$DATA_DIR/minio"

    # Generate process-compose config
    cp -f ${processComposeConfig} "$DATA_DIR/process-compose.yaml"

    # Process-compose unix socket path
    export PC_SOCKET="$DATA_DIR/process-compose.sock"

    # Export port file paths so other services can read the dynamic ports
    export RABBITMQ_AMQP_PORT_FILE="$DATA_DIR/rabbitmq/amqp_port"
    export RABBITMQ_MGMT_PORT_FILE="$DATA_DIR/rabbitmq/mgmt_port"

    # Postgres uses unix socket directly — socket dir is $DATA_DIR/postgres
    export PGHOST="$DATA_DIR/postgres"

    # Filesystem storage root (default backend)
    export STORAGE_BACKEND="fs"
    export STORAGE_FS_ROOT="$DATA_DIR/storage"
    mkdir -p "$STORAGE_FS_ROOT"

    # MinIO S3 port files
    export MINIO_API_PORT_FILE="$DATA_DIR/minio/api_port"
    export MINIO_CONSOLE_PORT_FILE="$DATA_DIR/minio/console_port"
  '';
}
