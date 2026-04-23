import os
import tempfile


def _s3_client():
    import boto3

    endpoint = os.environ["STORAGE_S3_ENDPOINT"]
    access_key = os.environ["STORAGE_S3_ACCESS_KEY"]
    secret_key = os.environ["STORAGE_S3_SECRET_KEY"]
    return boto3.client(
        "s3",
        endpoint_url=endpoint,
        aws_access_key_id=access_key,
        aws_secret_access_key=secret_key,
    )


def _s3_bucket(default: str = "engram") -> str:
    return os.environ.get("STORAGE_S3_BUCKET", default)


def get_file(file_path: str, storage_type: str) -> str:
    """Get the local path to a file for processing.

    For filesystem storage, returns the path directly.
    For S3 storage, downloads to a temp file and returns the temp path.
    """
    if storage_type == "fs":
        return file_path

    if storage_type == "s3":
        s3 = _s3_client()
        tmp = tempfile.NamedTemporaryFile(
            delete=False, suffix=os.path.splitext(file_path)[1]
        )
        tmp.close()
        s3.download_file(_s3_bucket(), file_path, tmp.name)
        return tmp.name

    raise ValueError(f"unknown storage type: {storage_type}")


def head_metadata(file_path: str, storage_type: str) -> dict[str, str]:
    """Return the user-metadata dict for an object.

    For filesystem storage, returns an empty dict. For S3, returns the
    lowercased x-amz-meta-* keys from head_object.
    """
    if storage_type != "s3":
        return {}

    s3 = _s3_client()
    resp = s3.head_object(Bucket=_s3_bucket(), Key=file_path)
    return resp.get("Metadata", {}) or {}
