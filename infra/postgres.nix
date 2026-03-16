{ pkgs, databases ? [ "engram" ] }:
let
  createDbCommands = builtins.concatStringsSep "\n" (map (db: ''
    if ! psql -h "$DATA_DIR/postgres" -d postgres -tc "SELECT 1 FROM pg_database WHERE datname = '${db}'" | grep -q 1; then
      echo "Creating database '${db}'..."
      createdb -h "$DATA_DIR/postgres" "${db}"
    else
      echo "Database '${db}' already exists."
    fi
  '') databases);
in
{
  processes = {
    postgres = {
      command = pkgs.writeShellScript "start-postgres" ''
        set -euo pipefail
        PGDATA="$DATA_DIR/postgres"

        if [ ! -f "$PGDATA/PG_VERSION" ]; then
          echo "Initializing PostgreSQL database..."
          initdb -D "$PGDATA" --no-locale --encoding=UTF8
          cat >> "$PGDATA/postgresql.conf" <<CONF
        unix_socket_directories = '$PGDATA'
        listen_addresses = '''
        CONF
        fi

        exec postgres -D "$PGDATA"
      '';
      readiness_probe = {
        exec.command = pkgs.writeShellScript "pg-ready" ''
          pg_isready -h "$DATA_DIR/postgres" -d postgres
        '';
        initial_delay_seconds = 2;
        period_seconds = 2;
      };
      shutdown = {
        command = pkgs.writeShellScript "stop-postgres" ''
          pg_ctl -D "$DATA_DIR/postgres" stop -m fast
        '';
      };
    };

    postgres-init = {
      command = pkgs.writeShellScript "init-databases" ''
        set -euo pipefail
        ${createDbCommands}
        echo "All databases ensured."
      '';
      depends_on.postgres.condition = "process_healthy";
      availability.restart = "no";
    };
  };

  socketDir = "$DATA_DIR/postgres";
}
