import logging
import os

from . import db, extractors, storage, tagger

log = logging.getLogger("engram-worker")


def process(file_id: str, file_path: str, filename: str, storage_type: str):
    tmp_path = None
    try:
        tmp_path = storage.get_file(file_path, storage_type)

        mime_type = extractors.detect_mime(tmp_path)
        log.info("file_id=%s mime=%s", file_id, mime_type)

        page_count = None
        extracted_text = None

        if mime_type == "application/pdf":
            result = extractors.extract_pdf(tmp_path)
            extracted_text = result["text"]
            page_count = result["page_count"]
        elif mime_type.startswith("image/"):
            extractors.extract_image(tmp_path)
        elif mime_type.startswith("text/"):
            result = extractors.extract_text(tmp_path)
            extracted_text = result["text"]

        tags = tagger.generate_tags(filename, mime_type)

        db.update_file_metadata(file_id, mime_type, page_count, extracted_text, tags)
        log.info("file_id=%s status=ready tags=%s", file_id, tags)

    finally:
        # Only clean up temp files (S3 downloads), not original fs files
        if tmp_path and storage_type == "s3" and os.path.exists(tmp_path):
            os.unlink(tmp_path)
