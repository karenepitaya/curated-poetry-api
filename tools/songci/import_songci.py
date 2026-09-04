"""Import the pinned electronic Song Ci selection; never fetch or OCR pages."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import unicodedata

from opencc import OpenCC

ROOT = Path(__file__).resolve().parents[2]
EDITION = "chinese-poetry-songci-b8594f8"
COLLECTION = "songci-digital-selection"
CONVERSION = "opencc-python-reimplemented@0.1.7:s2t"


def encode(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8")


def split_stanzas(lines, boundary):
    text = "".join(line["hans"] for line in lines)
    if hashlib.sha256(text.encode()).hexdigest() != boundary["textSHA256"]:
        raise ValueError("stanza source SHA-256 mismatch")
    cuts = boundary["breakAfterLine"]
    if cuts != sorted(set(cuts)) or any(type(cut) is not int or not 0 < cut < len(lines) for cut in cuts):
        raise ValueError("invalid stanza boundaries")
    starts, ends = [0, *cuts], [*cuts, len(lines)]
    return [{"id": f"stanza-{i + 1}", "kind": "stanza", "lines": lines[start:end]}
            for i, (start, end) in enumerate(zip(starts, ends))]


def han(char):
    return unicodedata.name(char, "").startswith(("CJK UNIFIED IDEOGRAPH", "CJK COMPATIBILITY IDEOGRAPH"))


def literary(text):
    return isinstance(text, str) and any(han(c) for c in text) and all(han(c) or unicodedata.category(c).startswith("P") for c in text)


def rejection(record):
    if set(record) != {"author", "paragraphs", "rhythmic", "tags"}:
        return "unexpected source fields"
    author = record["author"]
    if not isinstance(author, str) or len(author) < 2 or author == "赵令" or not all(han(c) for c in author):
        return "incomplete or invalid author name; requires source clarification"
    if not literary(record["rhythmic"]):
        return "invalid tune"
    paragraphs = record["paragraphs"]
    if not isinstance(paragraphs, list) or not paragraphs or any(not literary(p) for p in paragraphs):
        return "empty or invalid paragraph"
    if any(c in "□�■○" for p in paragraphs for c in p):
        return "unresolved missing-character marker"
    if any(p[-1] not in "。！？；" for p in paragraphs):
        return "paragraph lacks terminal punctuation; possible truncation"
    return None


def generate(root=ROOT):
    edition_path = root / "corpus/editions" / f"{EDITION}.json"
    edition = json.loads(edition_path.read_text(encoding="utf-8"))
    raw = (root / edition["sourcePath"]).read_bytes()
    digest = hashlib.sha256(raw).hexdigest()
    if digest != edition["sha256"]:
        raise ValueError("source SHA-256 mismatch")
    records = json.loads(raw)
    boundaries = json.loads((root / "sources/chinese-poetry-songci/stanzas.json").read_text(encoding="utf-8"))["works"]
    converter = OpenCC("s2t")
    def localized(text):
        return {"hans": text, "hant": converter.convert(text)}
    outputs, members, rejected = {}, [], []
    identities, contents = set(), set()
    for index, record in enumerate(records):
        reason = rejection(record)
        if reason:
            rejected.append({"recordIndex": index, "author": record.get("author"), "tune": record.get("rhythmic"), "reason": reason})
            continue
        first = re.split("[，。！？；、]", record["paragraphs"][0])[0]
        title = record["rhythmic"] + "·" + first
        identity = record["author"] + "\0" + title
        content = "".join(c for p in record["paragraphs"] for c in p if han(c))
        if identity in identities or content in contents:
            rejected.append({"recordIndex": index, "author": record["author"], "tune": record["rhythmic"], "reason": "duplicate identity or content"})
            continue
        identities.add(identity)
        contents.add(content)
        work_id = "song-digital-" + hashlib.sha256(identity.encode()).hexdigest()[:16]
        attribution = "unknown" if record["author"] == "无名氏" else "selected-edition"
        author = {"name": localized(record["author"]), "attributionStatus": attribution}
        if attribution == "unknown":
            author["attributionNote"] = "电子来源署名无名氏，未确定作者。"
        work = {
            "id": work_id, "title": localized(title), "author": author,
            "dynasty": "song", "genre": "ci", "form": "ci", "meter": "mixed",
            "tune": localized(record["rhythmic"]),
            "sections": split_stanzas([
                {"id": f"line-{i + 1}", **localized(p)} for i, p in enumerate(record["paragraphs"])
            ], boundaries[work_id]),
            "collections": [{"id": COLLECTION, "positionStatus": "pending"}],
            "evidence": {
                "level": "digital-text-checked", "status": "validated", "witnesses": [], "variants": [],
                "reviewedAt": edition["accessedAt"],
                "reviewMethod": "固定电子来源字句保留；按 sources/chinese-poetry-songci/stanzas.json 的电子文本标记核对分片；结构、字符与去重检查；繁体由 OpenCC s2t 生成，未作扫描校勘。",
                "digitalSource": {"editionId": EDITION, "recordIndex": index, "conversion": CONVERSION},
            },
        }
        outputs[f"corpus/works/song/{work_id}.json"] = encode(work)
        members.append({"workId": work_id, "positionStatus": "pending"})
    outputs[f"corpus/collections/{COLLECTION}.json"] = encode({
        "id": COLLECTION, "title": {"hans": "宋词电子文本选集", "hant": "宋詞電子文本選集"},
        "status": "in-progress", "primaryEditionId": EDITION, "members": members,
    })
    report = {"sourceSHA256": digest, "sourceRecords": len(records), "imported": len(members), "quarantined": len(rejected), "rejected": rejected,
              "limitations": ["Not a complete historical edition; upstream file contains 280 records.", "Source lines retained; stanza boundaries reviewed against electronic references in stanzas.json; original prefaces are not reconstructed.", "Hans retains source spellings, including mixed traditional forms; Hant is generated with OpenCC s2t.", "Mechanical validation does not prove historical textual accuracy."]}
    outputs["sources/chinese-poetry-songci/import-report.json"] = encode(report)
    return outputs, report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="verify committed outputs without writing")
    args = parser.parse_args()
    outputs, report = generate()
    expected = {ROOT / p for p in outputs if p.startswith("corpus/works/song/")}
    stale = set((ROOT / "corpus/works/song").glob("song-digital-*.json")) - expected
    if stale:
        raise SystemExit("Unexpected generated files; reconcile explicitly: " + ", ".join(str(p) for p in sorted(stale)))
    mismatches = []
    for name, content in outputs.items():
        path = ROOT / name
        if args.check:
            if not path.exists() or path.read_bytes() != content:
                mismatches.append(name)
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(content)
    if mismatches:
        raise SystemExit("Generated output differs: " + ", ".join(mismatches))
    print(f"Song Ci: source={report['sourceRecords']} imported={report['imported']} quarantined={report['quarantined']}; " + ("reproducibility checked" if args.check else "written"))


if __name__ == "__main__":
    main()
