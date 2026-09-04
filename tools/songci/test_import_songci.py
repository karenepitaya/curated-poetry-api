import copy
import tempfile
import unittest
from pathlib import Path

from import_songci import ROOT, generate, rejection


class ImportTests(unittest.TestCase):
    def test_reproducible_complete_outputs(self):
        outputs, report = generate()
        self.assertEqual((report["sourceRecords"], report["imported"], report["quarantined"]), (280, 276, 4))
        for name, raw in outputs.items():
            self.assertEqual((ROOT / name).read_bytes(), raw, name)

    def test_rejects_incomplete_records(self):
        valid = {"author": "苏轼", "rhythmic": "水调歌头", "paragraphs": ["明月几时有，把酒问青天。"], "tags": ["宋词三百首"]}
        self.assertIsNone(rejection(valid))
        for field, value in [("author", "韩"), ("author", "赵令"), ("paragraphs", []), ("paragraphs", ["明月□时有。"]), ("paragraphs", ["明月几时有"]), ("paragraphs", ["明月x时有。"])]:
            record = copy.deepcopy(valid)
            record[field] = value
            self.assertIsNotNone(rejection(record), (field, value))

    def test_source_hash_is_enforced_before_generation(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            edition = root / "corpus/editions/chinese-poetry-songci-b8594f8.json"
            edition.parent.mkdir(parents=True)
            edition.write_bytes((ROOT / edition.relative_to(root)).read_bytes())
            source = root / "sources/chinese-poetry-songci/songci-300.json"
            source.parent.mkdir(parents=True)
            source.write_bytes(b"[]")
            with self.assertRaisesRegex(ValueError, "SHA-256 mismatch"):
                generate(root)


if __name__ == "__main__":
    unittest.main()
