# To-do CLI in Python

Create a command-line to-do app in a single file `todo.py`, standard library
only (use `argparse`). Tasks persist to a `todos.json` file in the working
directory between runs.

Subcommands:
- `add "<text>"` — add a new task; print a confirmation.
- `list` — print all tasks, one per line, showing whether each is done.
- `done <n>` — mark task number `n` as done.

Example session:

```
python3 todo.py add "buy milk"
python3 todo.py list        # shows: 1. [ ] buy milk
python3 todo.py done 1
python3 todo.py list        # shows: 1. [x] buy milk
```

It must run with `python3 todo.py ...` and persist across invocations.
