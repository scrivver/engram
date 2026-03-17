import logging
import os
import signal
import time

import pika

from worker.handler import on_message

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("engram-worker")

QUEUE_NAME = "engram.ingest"
MAX_RETRY_DELAY = 30

_shutdown = False


def main():
    global _shutdown

    amqp_port = os.environ.get("RABBITMQ_AMQP_PORT", "5672")
    pg_host = os.environ.get("PGHOST", "/tmp")
    storage_backend = os.environ.get("STORAGE_BACKEND", "fs")

    log.info(
        "Starting engram ingestion worker (PGHOST=%s, RABBITMQ_AMQP_PORT=%s, STORAGE=%s)",
        pg_host,
        amqp_port,
        storage_backend,
    )

    def shutdown(signum, frame):
        global _shutdown
        _shutdown = True
        log.info("Shutdown requested...")

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    params = pika.ConnectionParameters(
        host="127.0.0.1",
        port=int(amqp_port),
        heartbeat=60,
        blocked_connection_timeout=300,
    )

    retry_delay = 1
    while not _shutdown:
        try:
            connection = pika.BlockingConnection(params)
            channel = connection.channel()

            channel.queue_declare(queue=QUEUE_NAME, durable=True)
            channel.basic_qos(prefetch_count=1)
            channel.basic_consume(queue=QUEUE_NAME, on_message_callback=on_message)

            retry_delay = 1  # Reset on successful connection
            log.info("Connected to RabbitMQ, listening on queue '%s'", QUEUE_NAME)

            while not _shutdown:
                # Process events with a timeout so we can check _shutdown
                connection.process_data_events(time_limit=1)

            log.info("Shutting down gracefully...")
            channel.stop_consuming()
            connection.close()
            break

        except pika.exceptions.AMQPConnectionError as e:
            log.warning("RabbitMQ connection failed: %s (retrying in %ds)", e, retry_delay)
            time.sleep(retry_delay)
            retry_delay = min(retry_delay * 2, MAX_RETRY_DELAY)

        except pika.exceptions.ConnectionClosedByBroker as e:
            log.warning("RabbitMQ connection closed by broker: %s (retrying in %ds)", e, retry_delay)
            time.sleep(retry_delay)
            retry_delay = min(retry_delay * 2, MAX_RETRY_DELAY)

        except pika.exceptions.StreamLostError:
            log.warning("RabbitMQ stream lost (retrying in %ds)", retry_delay)
            time.sleep(retry_delay)
            retry_delay = min(retry_delay * 2, MAX_RETRY_DELAY)

        except Exception as e:
            log.error("Unexpected error: %s (retrying in %ds)", e, retry_delay)
            time.sleep(retry_delay)
            retry_delay = min(retry_delay * 2, MAX_RETRY_DELAY)


if __name__ == "__main__":
    main()
