import logging
import os
import signal
import sys

import pika

from worker.handler import on_message

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("engram-worker")

QUEUE_NAME = "engram.ingest"


def main():
    amqp_port = os.environ.get("RABBITMQ_AMQP_PORT", "5672")
    pg_host = os.environ.get("PGHOST", "/tmp")
    storage_backend = os.environ.get("STORAGE_BACKEND", "fs")

    log.info(
        "Starting engram ingestion worker (PGHOST=%s, RABBITMQ_AMQP_PORT=%s, STORAGE=%s)",
        pg_host,
        amqp_port,
        storage_backend,
    )

    params = pika.ConnectionParameters(host="127.0.0.1", port=int(amqp_port))
    connection = pika.BlockingConnection(params)
    channel = connection.channel()

    channel.queue_declare(queue=QUEUE_NAME, durable=True)
    channel.basic_qos(prefetch_count=1)
    channel.basic_consume(queue=QUEUE_NAME, on_message_callback=on_message)

    def shutdown(signum, frame):
        log.info("Shutting down...")
        channel.stop_consuming()

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    log.info("Listening on queue '%s'", QUEUE_NAME)
    channel.start_consuming()
    connection.close()


if __name__ == "__main__":
    main()
