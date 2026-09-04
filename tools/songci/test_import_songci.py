import copy
import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from import_songci import ROOT, generate, rejection, split_stanzas


class ImportTests(unittest.TestCase):
    def test_stanzas_preserve_every_source_line_and_id(self):
        outputs, _ = generate()
        source = json.loads((ROOT / "sources/chinese-poetry-songci/songci-300.json").read_text(encoding="utf-8"))
        counts = set()
        for name, raw in outputs.items():
            if not name.startswith("corpus/works/song/"):
                continue
            work = json.loads(raw)
            lines = [line for section in work["sections"] for line in section["lines"]]
            record = source[work["evidence"]["digitalSource"]["recordIndex"]]
            self.assertEqual([line["hans"] for line in lines], record["paragraphs"])
            self.assertEqual([line["id"] for line in lines], [f"line-{i + 1}" for i in range(len(lines))])
            counts.add(len(work["sections"]))
            if work["title"]["hans"] == "安公子·弱柳丝千缕":
                self.assertEqual(len(work["sections"]), 2)
                self.assertEqual(work["sections"][1]["lines"][0]["hans"], "庾信愁如许。")
            if work["title"]["hans"] == "齐天乐·庾郎先自吟愁赋":
                self.assertEqual(work["sections"][1]["lines"][0]["hans"], "西窗又吹暗雨。")
        self.assertEqual(counts, {1, 2, 3, 4})

    def test_stanza_metadata_rejects_changed_text_and_invalid_boundaries(self):
        lines = [{"hans": "甲。"}, {"hans": "乙。"}]
        boundary = {"textSHA256": hashlib.sha256("甲。乙。".encode()).hexdigest(), "breakAfterLine": [1]}
        self.assertEqual(len(split_stanzas(lines, boundary)), 2)
        for cuts in [[0], [2], [1, 1]]:
            with self.assertRaisesRegex(ValueError, "invalid stanza"):
                split_stanzas(lines, {**boundary, "breakAfterLine": cuts})
        with self.assertRaisesRegex(ValueError, "SHA-256 mismatch"):
            split_stanzas([{"hans": "丙。"}, lines[1]], boundary)

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
