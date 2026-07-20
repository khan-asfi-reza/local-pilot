Build a small Python temperature-conversion library. Use ONLY the Python standard library — no third-party packages.

Create exactly these two files in the working directory:

1. `convert.py` — a module with two functions:
   - `c_to_f(c)` : convert Celsius to Fahrenheit, returns `c * 9/5 + 32`.
   - `f_to_c(f)` : convert Fahrenheit to Celsius, returns `(f - 32) * 5/9`.

2. `test_convert.py` — a plain-Python test script (no pytest) that imports the two functions from `convert` and checks, using `assert`:
   - `c_to_f(0) == 32`
   - `c_to_f(100) == 212`
   - `f_to_c(32) == 0`
   - `f_to_c(212) == 100`
   It must print `all tests passed` at the end if every assert passes.

Then verify it works by running `python3 test_convert.py` and confirm it prints `all tests passed`. Do not create any other files.
