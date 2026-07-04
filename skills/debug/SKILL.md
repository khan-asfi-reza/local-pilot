---
name: debug
description: Fix a failing program. Use when a command errors, a test fails, or the user pastes a traceback. Reproduce, read the exact file the error points to, apply one minimal fix, and re-run until it passes.
---

# Debugging workflow

Follow these steps in order. Do not skip reading.

1. Reproduce. Run the failing command (the one from the error, or the project's test/run command) with shell_run and read the structured result: exit_code, stdout, stderr.

2. Localize. Find the file and line the error names. Read that file with read_file before touching it. Never edit a file from memory or a guess of its contents. If the error names a symbol or import, read the file that defines it too.

3. Understand. State in one sentence the actual cause you see in the code, not a guess. If the stack trace names a module, an attribute, or an import, check that the name really exists where it is used.

4. Fix. Make the smallest change that addresses the cause. Rewrite the whole file with write_file (you have just read it), rather than a partial edit. Change one thing at a time.

5. Verify. Re-run the exact command that failed. Read the new result. If it still fails, go back to step 2 with the new error. Do not repeat the same change; if a change did not help, try a different one.

6. Stop when the command exits cleanly. Do not claim it is fixed until you have seen it run without the error.

Rules:
- Read before you change. Always.
- One change, then re-run. No blind rewrites.
- Fix the cause named by the error, not something unrelated.
- If two attempts do not move the error, re-read the file and the full error carefully before a third.
