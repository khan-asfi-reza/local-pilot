# Palindrome module in Node.js

Create a Node.js module, no dependencies.

Create exactly two files:
- `palindrome.js` — exports a function `isPalindrome(str)` using CommonJS
  (`module.exports = { isPalindrome }`). It ignores case, spaces, and
  punctuation, so `"A man a plan a canal Panama"` is a palindrome.
- `palindrome.test.js` — a test using the built-in `node:test` module that
  checks a few palindromes and non-palindromes.

`node --test` (or `node palindrome.test.js`) must pass.
