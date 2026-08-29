import json
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock, patch

from worker import handler


FIXTURES = Path(__file__).parents[2] / "contracts" / "file-events"


def fixture(name: str) -> bytes:
    return (FIXTURES / f"{name}.json").read_bytes()


class ParseMessageTest(unittest.TestCase):
    def test_canonical_contract_fixtures(self):
        for event in ("create", "delete", "rename"):
            with self.subTest(event=event):
                parsed = handler._parse_message(fixture(event))
                self.assertEqual(parsed["event"], event)

    def test_create_fixture_uses_display_path_contract(self):
        parsed = handler._parse_message(fixture("create"))

        self.assertEqual(parsed["filename"], "docs/myfile.pdf")
        self.assertEqual(parsed["file_path"], "files/alice/2026/07/docs/myfile.pdf")

    def test_rejects_unknown_event(self):
        message = json.loads(fixture("create"))
        message["event"] = "write"

        with self.assertRaisesRegex(ValueError, "invalid event"):
            handler._parse_message(json.dumps(message).encode())

    def test_rejects_non_sha256_create_hash(self):
        message = json.loads(fixture("create"))
        message["hash"] = "etag"

        with self.assertRaisesRegex(ValueError, "sha256"):
            handler._parse_message(json.dumps(message).encode())

    def test_keeps_legacy_minio_messages_during_migration(self):
        message = {
            "EventName": "s3:ObjectCreated:Put",
            "Records": [
                {
                    "eventTime": "2026-06-15T12:00:00Z",
                    "s3": {
                        "bucket": {"name": "reliquary"},
                        "object": {
                            "key": "files%2Falice%2Freport.pdf",
                            "size": 12,
                            "eTag": "legacy-etag",
                        },
                    },
                }
            ],
        }

        parsed = handler._parse_message(json.dumps(message).encode())

        self.assertEqual(parsed["file_path"], "files/alice/report.pdf")
        self.assertEqual(parsed["filename"], "report.pdf")
        self.assertEqual(parsed["hash"], "legacy-etag")


class OnMessageTest(unittest.TestCase):
    def setUp(self):
        self.channel = Mock()
        self.method = SimpleNamespace(delivery_tag=7)

    @patch.object(handler.pipeline, "process")
    @patch.object(handler.storage, "head_metadata", return_value={"owner": "alice"})
    @patch.object(handler.db, "update_file_status")
    @patch.object(handler.db, "upsert_file", return_value="file-id")
    def test_repeated_create_upserts_and_processes_same_identity(
        self,
        upsert_file,
        update_file_status,
        head_metadata,
        process,
    ):
        for _ in range(2):
            handler.on_message(self.channel, self.method, None, fixture("create"))

        self.assertEqual(upsert_file.call_count, 2)
        self.assertEqual(process.call_count, 2)
        upsert_file.assert_called_with(
            file_path="files/alice/2026/07/docs/myfile.pdf",
            filename="docs/myfile.pdf",
            size=204800,
            hash_value="sha256:abcdef123456",
            mtime="2026-07-12T12:00:00Z",
            device_name="reliquary",
            storage_type="s3",
            owner="alice",
        )
        self.assertEqual(update_file_status.call_count, 2)
        self.assertEqual(self.channel.basic_ack.call_count, 2)
        self.channel.basic_nack.assert_not_called()

    @patch.object(handler.db, "delete_file")
    def test_repeated_delete_is_acknowledged(self, delete_file):
        for _ in range(2):
            handler.on_message(self.channel, self.method, None, fixture("delete"))

        self.assertEqual(delete_file.call_count, 2)
        delete_file.assert_called_with(
            "s3",
            "files/alice/2026/06/report.pdf",
        )
        self.assertEqual(self.channel.basic_ack.call_count, 2)
        self.channel.basic_nack.assert_not_called()

    @patch.object(handler.db, "rename_file")
    def test_rename_is_scoped_by_storage_type(self, rename_file):
        handler.on_message(self.channel, self.method, None, fixture("rename"))

        rename_file.assert_called_once_with(
            "fs",
            "/srv/files/report.pdf",
            "/srv/files/report-final.pdf",
            "report-final.pdf",
        )
        self.channel.basic_ack.assert_called_once_with(delivery_tag=7)

    @patch.object(handler.db, "rename_file")
    def test_rename_preserves_folder_display_path(self, rename_file):
        message = json.loads(fixture("rename"))
        message.update(
            {
                "event": "rename",
                "file_path": "files/alice/2026/07/docs/myfile-final.pdf",
                "filename": "docs/myfile-final.pdf",
                "device_name": "reliquary",
                "storage_type": "s3",
                "old_file_path": "files/alice/2026/07/docs/myfile.pdf",
            }
        )

        handler.on_message(
            self.channel, self.method, None, json.dumps(message).encode()
        )

        rename_file.assert_called_once_with(
            "s3",
            "files/alice/2026/07/docs/myfile.pdf",
            "files/alice/2026/07/docs/myfile-final.pdf",
            "docs/myfile-final.pdf",
        )
        self.channel.basic_ack.assert_called_once_with(delivery_tag=7)


if __name__ == "__main__":
    unittest.main()
