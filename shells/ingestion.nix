{ pkgs, infraShell }:

pkgs.mkShell {
  name = "engram-ingestion-shell";
  inputsFrom = [ infraShell ];
  buildInputs = [
    (pkgs.python3.withPackages (ps: with ps; [
      pika        # RabbitMQ client
      psycopg2    # PostgreSQL client
      pip
    ]))
    pkgs.ruff     # Python linter/formatter
  ];
}
