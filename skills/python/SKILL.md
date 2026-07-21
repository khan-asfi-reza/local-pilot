---
name: python
description: Python project conventions.
internal: true
---
# Python
- Target Python 3. Follow PEP 8; use type hints on function signatures.
- Declare every dependency in `requirements.txt` (or `pyproject.toml`) under its real install name; never rely on a package being pre-installed.
- Prefer the standard library. Use a venv for installs.
- Verify before finishing: `python -c "import <module>"`, then run tests with `pytest -q` if tests exist.
- Don't start a blocking server with shell_run; import the module or run tests instead.
