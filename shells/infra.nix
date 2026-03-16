{ pkgs, processComposeConfig }:

pkgs.mkShell {
  name = "engram-infra-shell";
  buildInputs = [
    pkgs.rabbitmq-server
    pkgs.postgresql
    pkgs.process-compose
    pkgs.python3
    pkgs.curl
  ];

  shellHook = ''
    export SHELL=${pkgs.bash}/bin/bash
    export PATH="$PWD/bin:$PATH"

    export DATA_DIR="$PWD/.data"
    mkdir -p "$DATA_DIR"
    mkdir -p "$DATA_DIR/rabbitmq"
    mkdir -p "$DATA_DIR/postgres"

    # Generate process-compose config
    cp -f ${processComposeConfig} "$DATA_DIR/process-compose.yaml"

    # Process-compose unix socket path
    export PC_SOCKET="$DATA_DIR/process-compose.sock"

    # Export port file paths so other services can read the dynamic ports
    export RABBITMQ_AMQP_PORT_FILE="$DATA_DIR/rabbitmq/amqp_port"
    export RABBITMQ_MGMT_PORT_FILE="$DATA_DIR/rabbitmq/mgmt_port"

    # Postgres uses unix socket directly — socket dir is $DATA_DIR/postgres
    export PGHOST="$DATA_DIR/postgres"
  '';
}
