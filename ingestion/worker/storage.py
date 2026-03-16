import os
import tempfile


def get_file(file_path: str, storage_type: str) -> str:
    """Get the local path to a file for processing.

    For filesystem storage, returns the path directly.
    For S3 storage, downloads to a temp file and returns the temp path.
    """
    if storage_type == "fs":
        return file_path

    if storage_type == "s3":
        import boto3

        endpoint = os.environ["STORAGE_S3_ENDPOINT"]
        access_key = os.environ["STORAGE_S3_ACCESS_KEY"]
        secret_key = os.environ["STORAGE_S3_SECRET_KEY"]
        bucket = os.environ.get("STORAGE_S3_BUCKET", "engram")

        s3 = boto3.client(
            "s3",
            endpoint_url=endpoint,
            aws_access_key_id=access_key,
            aws_secret_access_key=secret_key,
        )
        tmp = tempfile.NamedTemporaryFile(
            delete=False, suffix=os.path.splitext(file_path)[1]
        )
        tmp.close()
        s3.download_file(bucket, file_path, tmp.name)
        return tmp.name

    raise ValueError(f"unknown storage type: {storage_type}")
