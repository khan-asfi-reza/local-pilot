"""Statement-coverage report for the Python side of Local Pilot (Shamsu).

The project's virtualenv has no coverage.py, so this uses the standard library's
trace module: run the suite with line tracing on, then compare the lines that
actually executed against the executable statements found by parsing each source
file. Run it from the repository root:

    backend/.venv/bin/python tests/python/coverage_report.py
"""

from __future__ import annotations

import ast
import io
import os
import sys
import threading
import trace
from contextlib import redirect_stdout
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
TARGETS = [
    REPO_ROOT / "backend" / "core",
    REPO_ROOT / "backend" / "services",
    REPO_ROOT / "backend" / "routes",
    REPO_ROOT / "telegram" / "bot.py",
]


def statement_lines(path: Path) -> set[int]:
    """Executable statement lines, ignoring imports, definitions and docstrings."""
    tree = ast.parse(path.read_text(), filename=str(path))
    lines: set[int] = set()
    for node in ast.walk(tree):
        if not isinstance(node, ast.stmt):
            continue
        if isinstance(node, (ast.Import, ast.ImportFrom, ast.FunctionDef,
                             ast.AsyncFunctionDef, ast.ClassDef, ast.Pass)):
            continue
        if isinstance(node, ast.Expr) and isinstance(node.value, ast.Constant) \
                and isinstance(node.value.value, str):
            continue  # docstring
        lines.add(node.lineno)
    return lines


def source_files() -> list[Path]:
    out: list[Path] = []
    for target in TARGETS:
        if target.is_file():
            out.append(target)
            continue
        for path in sorted(target.rglob("*.py")):
            if "__pycache__" in path.parts or path.name == "__init__.py":
                continue
            out.append(path)
    return out


def run_suite() -> dict[str, set[int]]:
    import pytest

    tracer = trace.Trace(count=1, trace=0, ignoredirs=[sys.prefix, sys.exec_prefix])
    # FastAPI's TestClient runs the app on a worker thread, and sys.settrace only
    # covers the thread that set it, so route handlers would otherwise look dead.
    threading.settrace(tracer.globaltrace)
    buffer = io.StringIO()
    with redirect_stdout(buffer):
        tracer.runfunc(pytest.main, ["-q", str(REPO_ROOT / "tests" / "python")])
    threading.settrace(None)
    print(buffer.getvalue().strip().splitlines()[-1])

    executed: dict[str, set[int]] = {}
    for (filename, lineno), hits in tracer.results().counts.items():
        if hits:
            executed.setdefault(os.path.realpath(filename), set()).add(lineno)
    return executed


def main() -> None:
    executed = run_suite()
    total_stmts = total_hit = 0
    rows = []
    for path in source_files():
        stmts = statement_lines(path)
        if not stmts:
            continue
        hit = stmts & executed.get(os.path.realpath(path), set())
        total_stmts += len(stmts)
        total_hit += len(hit)
        rows.append((str(path.relative_to(REPO_ROOT)), len(stmts), len(hit)))

    width = max(len(r[0]) for r in rows)
    print()
    print(f"{'file'.ljust(width)}  stmts   hit   cover")
    for name, stmts, hit in rows:
        print(f"{name.ljust(width)}  {stmts:5d} {hit:5d}  {100 * hit / stmts:5.1f}%")
    print(f"{'TOTAL'.ljust(width)}  {total_stmts:5d} {total_hit:5d}  {100 * total_hit / total_stmts:5.1f}%")


if __name__ == "__main__":
    main()
