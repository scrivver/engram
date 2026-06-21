{ pkgs }:

let
  python = pkgs.python313.withPackages (ps: with ps; [
    boto3
    pika
    pillow
    psycopg2
    pymupdf
    pytesseract
    python-magic
    reverse-geocode
  ]);
in
pkgs.stdenvNoCC.mkDerivation {
  pname = "engram-ingestion";
  version = "0.1.0";

  src = ../ingestion;

  nativeBuildInputs = [ pkgs.makeWrapper ];

  installPhase = ''
    runHook preInstall

    mkdir -p $out/lib/engram-ingestion $out/bin
    cp main.py $out/lib/engram-ingestion/
    cp -R worker $out/lib/engram-ingestion/

    makeWrapper ${python}/bin/python $out/bin/engram-ingestion \
      --add-flags "$out/lib/engram-ingestion/main.py" \
      --prefix PATH : ${pkgs.lib.makeBinPath [
        pkgs.file
        pkgs.ffmpeg
        pkgs.poppler-utils
        pkgs.tesseract
      ]} \
      --set-default SSL_CERT_FILE /etc/ssl/certs/ca-bundle.crt

    runHook postInstall
  '';

  meta = {
    description = "Engram Python metadata ingestion worker";
    mainProgram = "engram-ingestion";
  };
}
