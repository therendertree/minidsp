import json

from simulator.__main__ import main


def test_generate_only_prints_counts_and_hash(capsys):
    assert main(["--generate-only", "--seed", "42", "--count", "1000"]) == 0
    out = capsys.readouterr().out
    assert "requests=1000" in out and "requests_sha256=" in out
    assert "valid" in out and "size_mismatch" in out


def test_generate_only_is_reproducible(capsys):
    main(["--generate-only", "--seed", "7", "--count", "50"])
    first = capsys.readouterr().out
    main(["--generate-only", "--seed", "7", "--count", "50"])
    assert capsys.readouterr().out == first


def test_dump_writes_one_json_line_per_request(tmp_path, capsys):
    path = tmp_path / "out.jsonl"
    assert main(["--generate-only", "--count", "25", "--dump", str(path)]) == 0
    lines = path.read_text(encoding="utf-8").splitlines()
    assert len(lines) == 25
    assert {"index", "category", "kind", "expected_status", "body"} <= set(json.loads(lines[0]))


def test_negative_count_is_a_usage_error(capsys):
    assert main(["--generate-only", "--count", "-1"]) == 2
