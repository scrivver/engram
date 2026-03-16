import json
import logging
import traceback

from . import db, pipeline

log = logging.getLogger("engram-worker")


def on_message(channel, method, properties, body):
    try:
        msg = json.loads(body)
        file_id = msg["file_id"]
        log.info("Processing file_id=%s filename=%s", file_id, msg.get("filename"))

        db.update_file_status(file_id, "processing")

        pipeline.process(
            file_id=file_id,
            object_key=msg["object_key"],
            bucket=msg["storage_bucket"],
            filename=msg["filename"],
        )

        channel.basic_ack(delivery_tag=method.delivery_tag)

    except Exception:
        log.error("Failed to process message: %s", traceback.format_exc())
        try:
            file_id = json.loads(body).get("file_id")
            if file_id:
                db.update_file_status(file_id, "failed")
        except Exception:
            pass
        channel.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
