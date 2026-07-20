Build a small Go command-line program. Use ONLY the Go standard library.

Create exactly these two files in the working directory:

1. `go.mod` — module named `wordcount`, Go version `1.21`.

2. `main.go` — package main. The program takes one command-line argument: a path to a text file. It reads that file and prints, on one line separated by single spaces: the number of lines, the number of words, and the total number of characters (BYTES, including newline characters), then the filename. Format exactly: `<lines> <words> <chars> <filename>`. If no argument is given, print `usage: wordcount <file>` to stderr and exit with code 1.

Verify BEHAVIOR, not just compilation:
- Build with `go build ./...`.
- Then create a test file with exactly the two lines `one two` and `three` (so the file is the 14 bytes: `one two\nthree\n`).
- Run your program on it and confirm the output is exactly `2 3 14 <that filename>`. The character count MUST include the two newline bytes — if you get 12, your counting is wrong; fix it.
Only finish once the output is correct. Do not create any other files besides go.mod and main.go (a throwaway test text file is fine).
