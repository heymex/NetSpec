#!/usr/bin/env python3
import logging
import os
import tempfile
import unittest
from pathlib import Path

import mdt_to_netspec as forwarder


class DecodedTailTests(unittest.TestCase):
    def test_follows_rotation_to_new_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "decoded.json"
            path.write_text("old\n", encoding="utf-8")
            tail = forwarder.DecodedTail(path)
            self.assertEqual(tail.readline(), "")

            os.rename(path, path.parent / "decoded.json.1")
            path.write_text("new\n", encoding="utf-8")
            line = tail.readline()
            tail.close()
            self.assertEqual(line.strip(), "new")

    def test_prune_removes_numeric_archives_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            sidecar = Path(tmp)
            archive = sidecar / "decoded.json.1"
            keep = sidecar / "decoded.json.backup"
            archive.write_text("x" * 32, encoding="utf-8")
            keep.write_text("keep", encoding="utf-8")
            logger = logging.getLogger("test-prune")
            removed = forwarder.prune_decoded_archives(sidecar, logger)
            self.assertEqual(removed, 1)
            self.assertFalse(archive.exists())
            self.assertTrue(keep.exists())


if __name__ == "__main__":
    unittest.main()
