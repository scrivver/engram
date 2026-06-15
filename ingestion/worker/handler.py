import json
import logging
import traceback
from urllib.parse import unquote_plus

from . import db, pipeline, storage

log = logging.getLogger("engram-worker")

EVENT_TYPES = {"create", "delete", "rename"}
STORAGE_TYPES = {"fs", "s3"}


def _validate_message(msg: dict) -> dict:
    event = msg.get("event")
    if event not in EVENT_TYPES:
        raise ValueError(f"invalid event: {event!r}")

    for field in ("file_path", "filename", "device_name", "storage_type"):
        if not msg.get(field):
            raise ValueError(f"{field} is required")

    if msg["storage_type"] not in STORAGE_TYPES:
        raise ValueError(f"invalid storage_type: {msg['storage_type']!r}")

    if event in {"create", "rename"}:
        for field in ("hash", "mtime"):
            if not msg.get(field):
                raise ValueError(f"{field} is required for {event}")
        if not str(msg["hash"]).startswith("sha256:"):
            raise ValueError("hash must use the sha256:<hex> format")
        if not isinstance(msg.get("size"), int) or msg["size"] < 0:
            raise ValueError(f"size must be a non-negative integer for {event}")

    if event == "rename" and not msg.get("old_file_path"):
        raise ValueError("old_file_path is required for rename")

    return msg


def _parse_message(body: bytes) -> dict:
    """Parse message and normalize into common format.

    Handles two message types:
    1. Filesystem watcher event (has "event" key)
    2. S3 bucket notification (has "EventName" or "Records" key)
    """
    msg = json.loads(body)

    # Filesystem watcher event
    if "event" in msg:
        return _validate_message(
            {
                "event": msg["event"],
                "file_path": msg.get("file_path", ""),
                "filename": msg.get("filename", ""),
                "size": msg.get("size", 0),
                "hash": msg.get("hash", ""),
                "mtime": msg.get("mtime", ""),
                "device_name": msg.get("device_name", ""),
                "storage_type": msg.get("storage_type", "fs"),
                "old_file_path": msg.get("old_file_path", ""),
            }
        )

    # S3 bucket notification (MinIO format)
    if "EventName" in msg or "Records" in msg:
        records = msg.get("Records", [msg])
        record = records[0]
        s3_info = record.get("s3", {})
        bucket = s3_info.get("bucket", {}).get("name", "")
        obj = s3_info.get("object", {})
        raw_key = obj.get("key", msg.get("Key", ""))
        # S3 notifications URL-encode the key (e.g. spaces as '+', '/' as '%2F').
        key = unquote_plus(raw_key)
        size = obj.get("size", 0)

        filename = key.rsplit("/", 1)[-1] if "/" in key else key

        event_name = msg.get("EventName", record.get("eventName", ""))
        if "ObjectRemoved" in event_name:
            event = "delete"
        else:
            event = "create"

        return {
            "event": event,
            "file_path": key,
            "filename": filename,
            "size": size,
            "hash": obj.get("eTag", ""),
            "mtime": record.get("eventTime", ""),
            "device_name": bucket or "s3",
            "storage_type": "s3",
            "old_file_path": "",
        }

    raise ValueError(f"unknown message format: {list(msg.keys())}")


def on_message(channel, method, properties, body):
    file_id = None
    try:
        parsed = _parse_message(body)
        event = parsed["event"]

        # Handle delete events
        if event == "delete":
            log.info("Delete event: %s", parsed["file_path"])
            db.delete_file(parsed["storage_type"], parsed["file_path"])
            channel.basic_ack(delivery_tag=method.delivery_tag)
            return

        # Handle rename events
        if event == "rename":
            log.info(
                "Rename event: %s -> %s", parsed["old_file_path"], parsed["file_path"]
            )
            db.rename_file(
                parsed["storage_type"],
                parsed["old_file_path"],
                parsed["file_path"],
                parsed["filename"],
            )
            channel.basic_ack(delivery_tag=method.delivery_tag)
            return

        # Handle create/write events
        log.info(
            "Processing %s (storage=%s, device=%s)",
            parsed["filename"],
            parsed["storage_type"],
            parsed["device_name"],
        )

        # Fetch object metadata to derive ownership (stamped as x-amz-meta-owner
        # on upload). boto3 lowercases user-metadata keys.
        meta = storage.head_metadata(parsed["file_path"], parsed["storage_type"])
        owner = meta.get("owner") or None

        file_id = db.upsert_file(
            file_path=parsed["file_path"],
            filename=parsed["filename"],
            size=parsed["size"],
            hash_value=parsed["hash"],
            mtime=parsed["mtime"],
            device_name=parsed["device_name"],
            storage_type=parsed["storage_type"],
            owner=owner,
        )

        db.update_file_status(file_id, "processing")

        pipeline.process(
            file_id=file_id,
            file_path=parsed["file_path"],
            filename=parsed["filename"],
            storage_type=parsed["storage_type"],
            mtime=parsed.get("mtime"),
        )

        channel.basic_ack(delivery_tag=method.delivery_tag)

    except Exception:
        log.error("Failed to process message: %s", traceback.format_exc())
        try:
            if file_id:
                db.update_file_status(file_id, "failed")
        except Exception:
            pass
        channel.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
