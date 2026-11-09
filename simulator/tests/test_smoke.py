from simulator.__main__ import main


def test_main_runs(capsys):
    assert main() == 0
    assert "skeleton" in capsys.readouterr().out
