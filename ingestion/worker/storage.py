import os
import shutil
import tempfile

_backend = os.environ.get("STORAGE_BACKEND", "fs")
_fs_root = os.environ.get("STORAGE_FS_ROOT", ".data/storage")


def download_file(object_key: str, bucket: str) -> str:
    """Download file from storage to a temp path. Returns the temp file path."""
    if _backend == "fs":
        src = os.path.join(_fs_root, object_key)
        tmp = tempfile.NamedTemporaryFile(delete=False, suffix=os.path.splitext(object_key)[1])
        shutil.copy2(src, tmp.name)
        tmp.close()
        return tmp.name

    if _backend == "s3":
        import boto3

        endpoint = os.environ["STORAGE_S3_ENDPOINT"]
        access_key = os.environ["STORAGE_S3_ACCESS_KEY"]
        secret_key = os.environ["STORAGE_S3_SECRET_KEY"]

        s3 = boto3.client(
            "s3",
            endpoint_url=endpoint,
            aws_access_key_id=access_key,
            aws_secret_access_key=secret_key,
        )
        tmp = tempfile.NamedTemporaryFile(delete=False, suffix=os.path.splitext(object_key)[1])
        tmp.close()
        s3.download_file(bucket, object_key, tmp.name)
        return tmp.name

    raise ValueError(f"unknown storage backend: {_backend}")
