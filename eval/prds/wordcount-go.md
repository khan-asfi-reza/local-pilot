# Word count CLI in Go

Build a small command-line program in Go, using only the standard library.

Create exactly two files:
- `go.mod` (module name `wc`)
- `main.go`

The program takes one argument, a filename, and prints a single line:

```
<lines> <words> <bytes> <filename>
```

- `<lines>` = number of newline characters in the file
- `<words>` = number of whitespace-separated words
- `<bytes>` = total number of bytes in the file
- `<filename>` = the filename argument, exactly as given

For example, for a file whose contents are `a b\nc\n` the output must be:

```
2 3 6 sample.txt
```

It must build with `go build` and run correctly.
