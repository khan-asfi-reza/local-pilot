Build a small Node.js string utility. Use ONLY built-in Node — no npm packages, no package.json.

Create exactly these two files in the working directory:

1. `slugify.js` — exports a single function `slugify(str)` via CommonJS (`module.exports = slugify`). The function must:
   - lowercase the string,
   - trim leading/trailing whitespace,
   - replace any run of non-alphanumeric characters with a single hyphen `-`,
   - remove leading/trailing hyphens.
   Example: `slugify("  Hello, World!  ")` returns `"hello-world"`.

2. `test.js` — a plain Node script (no test framework) that `require`s `./slugify` and checks with `assert` (built-in `node:assert`):
   - `slugify("  Hello, World!  ") === "hello-world"`
   - `slugify("Foo   Bar") === "foo-bar"`
   - `slugify("already-slug") === "already-slug"`
   It must print `all tests passed` at the end if every assert passes.

Then verify it works by running `node test.js` and confirm it prints `all tests passed`. Do not create any other files.
