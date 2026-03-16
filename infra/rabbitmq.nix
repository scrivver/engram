{ pkgs }:
{
  processes = {
    rabbitmq = {
      command = pkgs.writeShellScript "start-rabbitmq" ''
        set -euo pipefail

        RABBITMQ_DIR="$DATA_DIR/rabbitmq"
        mkdir -p "$RABBITMQ_DIR"

        # Pick an ephemeral port for AMQP and management
        AMQP_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
        MGMT_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
        echo "$AMQP_PORT" > "$RABBITMQ_DIR/amqp_port"
        echo "$MGMT_PORT" > "$RABBITMQ_DIR/mgmt_port"

        export RABBITMQ_MNESIA_BASE="$RABBITMQ_DIR/mnesia"
        export RABBITMQ_LOG_BASE="$RABBITMQ_DIR/log"
        export RABBITMQ_SCHEMA_DIR="$RABBITMQ_DIR/schema"
        export RABBITMQ_GENERATED_CONFIG_DIR="$RABBITMQ_DIR/config"
        export RABBITMQ_NODE_PORT="$AMQP_PORT"
        export RABBITMQ_NODENAME="engram@localhost"
        export RABBITMQ_PLUGINS_DIR="${pkgs.rabbitmq-server}/plugins"
        export RABBITMQ_ENABLED_PLUGINS_FILE="$RABBITMQ_DIR/enabled_plugins"

        mkdir -p "$RABBITMQ_MNESIA_BASE" "$RABBITMQ_LOG_BASE" "$RABBITMQ_SCHEMA_DIR" "$RABBITMQ_GENERATED_CONFIG_DIR"

        # Enable management plugin
        echo '[rabbitmq_management].' > "$RABBITMQ_ENABLED_PLUGINS_FILE"

        # Write config
        cat > "$RABBITMQ_DIR/rabbitmq.conf" <<RMQEOF
        listeners.tcp.default = $AMQP_PORT
        management.tcp.port = $MGMT_PORT
        default_user = guest
        default_pass = guest
        loopback_users = none
        RMQEOF

        export RABBITMQ_CONFIG_FILE="$RABBITMQ_DIR/rabbitmq"

        echo "RabbitMQ starting on AMQP :$AMQP_PORT, Management :$MGMT_PORT"

        exec ${pkgs.rabbitmq-server}/bin/rabbitmq-server
      '';
      readiness_probe = {
        exec.command = pkgs.writeShellScript "rabbitmq-ready" ''
          AMQP_PORT=$(cat "$DATA_DIR/rabbitmq/amqp_port" 2>/dev/null) || exit 1
          ${pkgs.rabbitmq-server}/bin/rabbitmqctl --node engram@localhost status >/dev/null 2>&1
        '';
        initial_delay_seconds = 5;
        period_seconds = 3;
      };
    };
  };
}
