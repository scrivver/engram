import json
import logging
import traceback

from . import db, pipeline

log = logging.getLogger("engram-worker")


def _parse_message(body: bytes) -> dict:
    """Parse message and normalize into common format.

    Handles two message types:
    1. Filesystem watcher event (has "event" key)
    2. S3 bucket notification (has "EventName" or "Records" key)
    """
    msg = json.loads(body)

    # Filesystem watcher event
    if "event" in msg:
        return {
            "file_path": msg["file_path"],
            "filename": msg["filename"],
            "size": msg["size"],
            "hash": msg["hash"],
            "mtime": msg["mtime"],
            "device_name": msg["device_name"],
            "storage_type": msg["storage_type"],
        }

    # S3 bucket notification (MinIO format)
    if "EventName" in msg or "Records" in msg:
        records = msg.get("Records", [msg])
        record = records[0]
        s3_info = record.get("s3", {})
        bucket = s3_info.get("bucket", {}).get("name", "")
        obj = s3_info.get("object", {})
        key = obj.get("key", msg.get("Key", ""))
        size = obj.get("size", 0)

        # Use bucket name as device name for S3 sources
        filename = key.rsplit("/", 1)[-1] if "/" in key else key

        return {
            "file_path": key,
            "filename": filename,
            "size": size,
            "hash": obj.get("eTag", ""),
            "mtime": record.get("eventTime", ""),
            "device_name": bucket or "s3",
            "storage_type": "s3",
        }

    raise ValueError(f"unknown message format: {list(msg.keys())}")


def on_message(channel, method, properties, body):
    try:
        parsed = _parse_message(body)
        log.info(
            "Processing %s (storage=%s, device=%s)",
            parsed["filename"],
            parsed["storage_type"],
            parsed["device_name"],
        )

        # Insert file record and get file_id
        file_id = db.insert_file(
            file_path=parsed["file_path"],
            filename=parsed["filename"],
            size=parsed["size"],
            hash_value=parsed["hash"],
            mtime=parsed["mtime"],
            device_name=parsed["device_name"],
            storage_type=parsed["storage_type"],
        )

        if file_id is None:
            # Duplicate — already processed
            log.info("Duplicate file (hash=%s), skipping", parsed["hash"])
            channel.basic_ack(delivery_tag=method.delivery_tag)
            return

        db.update_file_status(file_id, "processing")

        pipeline.process(
            file_id=file_id,
            file_path=parsed["file_path"],
            filename=parsed["filename"],
            storage_type=parsed["storage_type"],
        )

        channel.basic_ack(delivery_tag=method.delivery_tag)

    except Exception:
        log.error("Failed to process message: %s", traceback.format_exc())
        try:
            # Try to mark as failed if we have a file_id
            if "file_id" in dir() and file_id:
                db.update_file_status(file_id, "failed")
        except Exception:
            pass
        channel.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
