{
  description = "Flake for the engram project, includes development environment and infrastructure service definitions.";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
	let
	pkgs = import nixpkgs {
	inherit system;
	config.allowUnfree = true;
	};
	rabbitmqInfra = import ./infra/rabbitmq.nix { inherit pkgs; };
	postgresInfra = import ./infra/postgres.nix { inherit pkgs; };
	yamlFormat = pkgs.formats.yaml {};
	processComposeConfig = yamlFormat.generate "process-compose.yaml" {
	  version = "0.5";
	  processes = rabbitmqInfra.processes // postgresInfra.processes;
	};
	infraShell     = import ./shells/infra.nix { inherit pkgs processComposeConfig; };
	backendShell   = import ./shells/backend.nix { inherit pkgs infraShell; };
	ingestionShell = import ./shells/ingestion.nix { inherit pkgs infraShell; };
	in
	{
	devShells = rec {
	  infra     = infraShell;
	  backend   = backendShell;
	  watcher   = backendShell;  # watcher uses same Go tools as backend
	  ingestion = ingestionShell;
	  full      = pkgs.mkShell {
	    name = "engram-full-shell";
	    inputsFrom = [ backendShell ingestionShell ];
	  };
	  default   = full;
	};
	}
  );
}
