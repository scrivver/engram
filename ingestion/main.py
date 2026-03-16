import logging
import os
import signal
import sys

import pika

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("engram-worker")


def on_message(channel, method, properties, body):
    log.info("Received message: %s", body.decode())
    channel.basic_ack(delivery_tag=method.delivery_tag)


def main():
    amqp_port = os.environ.get("RABBITMQ_AMQP_PORT", "5672")
    pg_host = os.environ.get("PGHOST", "/tmp")

    log.info("Starting engram ingestion worker (PGHOST=%s, RABBITMQ_AMQP_PORT=%s)", pg_host, amqp_port)

    params = pika.ConnectionParameters(host="127.0.0.1", port=int(amqp_port))
    connection = pika.BlockingConnection(params)
    channel = connection.channel()

    queue_name = "engram.ingest"
    channel.queue_declare(queue=queue_name, durable=True)
    channel.basic_qos(prefetch_count=1)
    channel.basic_consume(queue=queue_name, on_message_callback=on_message)

    def shutdown(signum, frame):
        log.info("Shutting down...")
        channel.stop_consuming()

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    log.info("Listening on queue '%s'", queue_name)
    channel.start_consuming()
    connection.close()


if __name__ == "__main__":
    main()
