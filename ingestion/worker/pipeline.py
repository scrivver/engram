import logging
import os

from . import db, extractors, storage, tagger

log = logging.getLogger("engram-worker")


def process(file_id: str, file_path: str, filename: str, storage_type: str, mtime: str | None = None):
    tmp_path = None
    try:
        tmp_path = storage.get_file(file_path, storage_type)

        mime_type = extractors.detect_mime(tmp_path)
        log.info("file_id=%s mime=%s", file_id, mime_type)

        page_count = None
        extracted_text = None
        gps = None
        media_info = None

        if mime_type == "application/pdf":
            result = extractors.extract_pdf(tmp_path)
            extracted_text = result["text"]
            page_count = result["page_count"]
        elif mime_type.startswith("image/"):
            result = extractors.extract_image(tmp_path)
            gps = result.get("gps")
        elif mime_type.startswith("text/"):
            result = extractors.extract_text(tmp_path)
            extracted_text = result["text"]
        elif mime_type.startswith("video/") or mime_type.startswith("audio/"):
            media_info = extractors.extract_media(tmp_path)

        tags = tagger.generate_tags(
            filename=filename,
            mime_type=mime_type,
            mtime=mtime,
            gps=gps,
            media_info=media_info,
        )

        db.update_file_metadata(file_id, mime_type, page_count, extracted_text, tags)
        log.info("file_id=%s status=ready tags=%s", file_id, tags)

    finally:
        # Only clean up temp files (S3 downloads), not original fs files
        if tmp_path and storage_type == "s3" and os.path.exists(tmp_path):
            os.unlink(tmp_path)
