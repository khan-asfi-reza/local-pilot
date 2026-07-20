Build a browser Snake game as a single self-contained file `index.html` (HTML + CSS + JavaScript all in one file, no external libraries, no build step).

Requirements:
- An HTML5 `<canvas>` (about 400x400) where the game is drawn.
- A snake that moves on a grid, controlled by the arrow keys.
- Food appears at a random grid cell; eating it grows the snake by one and increases the score by 1.
- The game ends if the snake hits a wall or itself; show a "Game Over" message and the final score, and allow restarting by pressing a key (e.g. Space or Enter).
- Display the current score on screen.
- Use `requestAnimationFrame` or `setInterval` for the game loop.

Keep everything in the one `index.html` file. Do not create any other files.

VERIFY: check that the JavaScript has no syntax errors (e.g. run `node --check` on the extracted script, or at minimum re-read the file and confirm the game loop, key handling, collision detection, and restart logic are all present and correct). Fix any error before finishing.
