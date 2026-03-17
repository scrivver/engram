{ pkgs, bucketName ? "engram" }:
{
  processes = {
    minio = {
      command = pkgs.writeShellScript "start-minio" ''
        set -euo pipefail

        MINIO_DIR="$DATA_DIR/minio"
        mkdir -p "$MINIO_DIR/data"
        rm -f "$MINIO_DIR/api_port" "$MINIO_DIR/console_port"

        API_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
        CONSOLE_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
        echo "$API_PORT" > "$MINIO_DIR/api_port"
        echo "$CONSOLE_PORT" > "$MINIO_DIR/console_port"

        export MINIO_ROOT_USER=minioadmin
        export MINIO_ROOT_PASSWORD=minioadmin

        # Configure AMQP notification to RabbitMQ
        AMQP_PORT=$(cat "$DATA_DIR/rabbitmq/amqp_port")
        export MINIO_NOTIFY_AMQP_ENABLE_PRIMARY=on
        export MINIO_NOTIFY_AMQP_URL_PRIMARY="amqp://guest:guest@127.0.0.1:$AMQP_PORT"
        export MINIO_NOTIFY_AMQP_EXCHANGE_PRIMARY="amq.direct"
        export MINIO_NOTIFY_AMQP_EXCHANGE_TYPE_PRIMARY="direct"
        export MINIO_NOTIFY_AMQP_ROUTING_KEY_PRIMARY="engram.ingest"
        export MINIO_NOTIFY_AMQP_DURABLE_PRIMARY=on
        export MINIO_NOTIFY_AMQP_NO_WAIT_PRIMARY=off
        export MINIO_NOTIFY_AMQP_AUTO_DELETED_PRIMARY=off

        echo "MinIO starting on API :$API_PORT, Console :$CONSOLE_PORT"
        echo "  AMQP notifications → RabbitMQ :$AMQP_PORT (queue: engram.ingest)"

        exec ${pkgs.minio}/bin/minio server \
          --address "127.0.0.1:$API_PORT" \
          --console-address "127.0.0.1:$CONSOLE_PORT" \
          "$MINIO_DIR/data"
      '';
      depends_on = {
        rabbitmq.condition = "process_healthy";
      };
      readiness_probe = {
        exec.command = pkgs.writeShellScript "minio-ready" ''
          API_PORT=$(cat "$DATA_DIR/minio/api_port" 2>/dev/null) || exit 1
          curl -sf "http://127.0.0.1:$API_PORT/minio/health/live" -o /dev/null 2>&1
        '';
        initial_delay_seconds = 3;
        period_seconds = 2;
      };
    };

    minio-setup = {
      command = pkgs.writeShellScript "minio-setup" ''
        set -euo pipefail

        API_PORT=$(cat "$DATA_DIR/minio/api_port")
        ENDPOINT="http://127.0.0.1:$API_PORT"

        ${pkgs.minio-client}/bin/mc alias set local "$ENDPOINT" minioadmin minioadmin --api S3v4

        # Create bucket
        ${pkgs.minio-client}/bin/mc mb --ignore-existing local/${bucketName}

        # Enable bucket notifications for object creation events
        ${pkgs.minio-client}/bin/mc event add local/${bucketName} arn:minio:sqs::PRIMARY:amqp --event put 2>/dev/null || true

        echo "MinIO bucket '${bucketName}' ready with AMQP notifications enabled"
        echo "  Endpoint: $ENDPOINT"
        echo "  Upload:   mc cp <file> local/${bucketName}/"
      '';
      depends_on = {
        minio.condition = "process_healthy";
      };
      availability = {
        restart = "no";
      };
    };
  };

  inherit bucketName;
}
