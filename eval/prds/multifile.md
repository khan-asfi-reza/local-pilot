Build a small Python package. Use ONLY the Python standard library.

Create exactly these files in the working directory:

1. `mathutils/__init__.py` — empty file.
2. `mathutils/ops.py` — defines two functions: `add(a, b)` returning `a + b`, and `mul(a, b)` returning `a * b`.
3. `main.py` — imports `add` and `mul` from `mathutils.ops`, then prints the result of `add(2, 3)` on the first line and `mul(2, 3)` on the second line.

Verify by running `python3 main.py` and confirm it prints exactly:
```
5
6
```
Only finish once the output is exactly those two lines. Do not create any other files.
