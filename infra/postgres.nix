{ pkgs, dbName ? "engram" }:
{
  processes = {
    postgres = {
      command = pkgs.writeShellScript "start-postgres" ''
        set -euo pipefail

        PG_DIR="$DATA_DIR/postgres"
        mkdir -p "$PG_DIR"

        PGDATA="$PG_DIR/data"

        # Initialise cluster if not present
        if [ ! -f "$PGDATA/PG_VERSION" ]; then
          ${pkgs.postgresql}/bin/initdb -D "$PGDATA" --no-locale --encoding=UTF8
          echo "unix_socket_directories = '$PG_DIR'" >> "$PGDATA/postgresql.conf"
          echo "listen_addresses = '''" >> "$PGDATA/postgresql.conf"
        fi

        echo "PostgreSQL starting on unix socket: $PG_DIR/.s.PGSQL.5432"

        exec ${pkgs.postgresql}/bin/postgres -D "$PGDATA" -k "$PG_DIR"
      '';
      readiness_probe = {
        exec.command = pkgs.writeShellScript "postgres-ready" ''
          PG_DIR="$DATA_DIR/postgres"
          ${pkgs.postgresql}/bin/pg_isready -h "$PG_DIR" -q
        '';
        initial_delay_seconds = 3;
        period_seconds = 2;
      };
    };

    postgres-create-db = {
      command = pkgs.writeShellScript "postgres-create-db" ''
        PG_DIR="$DATA_DIR/postgres"
        ${pkgs.postgresql}/bin/createdb -h "$PG_DIR" "${dbName}" 2>/dev/null || true
        echo "Database '${dbName}' ready on unix socket: $PG_DIR"
      '';
      depends_on = {
        postgres.condition = "process_healthy";
      };
      availability = {
        restart = "no";
      };
    };
  };

  inherit dbName;
}
