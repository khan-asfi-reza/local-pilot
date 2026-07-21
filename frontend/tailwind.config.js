/** @type {import('tailwindcss').Config} */

// Near-black identity: a cool, neutral almost-black surface set (like the first
// reference) with a blue -> violet -> fuchsia gradient accent (like the Create
// button). Components are written against zinc / emerald / teal, so those ramps
// are redefined here — every class shifts at once, keeping the app cohesive.
const neutral = {
  50: '#f4f4f5',
  100: '#e8e8ea', // primary text
  200: '#d1d1d4',
  300: '#a6a6ac', // secondary text
  400: '#83838a', // muted text
  500: '#6a6a71',
  600: '#48484e', // faint text
  700: '#26262a', // hairline borders
  800: '#1b1b1e', // borders / hover surfaces
  850: '#141416', // elevated panels
  900: '#0a0a0b', // app background (near-black)
  950: '#060607', // inset wells
};

// Accent gradient endpoints. `from-emerald-500 to-teal-600` becomes the blue ->
// fuchsia sweep from the reference button (it passes through violet in between).
// emerald = the blue/indigo start + violet single-accent; teal = the fuchsia end.
const indigo = {
  50: '#eef0fe',
  100: '#dce0fd',
  200: '#bcc3fb',
  300: '#9aa4f8',
  400: '#a78bfa', // single-accent (icons, text, checks) — violet
  500: '#6d5ef6', // gradient start — blue/indigo
  600: '#7c3aed', // solid accent — violet
  700: '#6d28d9',
  800: '#5b21b6',
  900: '#4c1d95',
  950: '#2e1065',
};

const fuchsia = {
  50: '#fdeafc',
  100: '#fbd5f9',
  200: '#f6abf1',
  300: '#ef7fe6',
  400: '#e05fd6',
  500: '#c13ad6', // magenta
  600: '#b026c9', // gradient end — fuchsia
  700: '#8f1ea6',
  800: '#6f1782',
  900: '#521263',
  950: '#340a40',
};

export default {
  content: ['./index.html', './src/**/*.{js,jsx,ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter Variable', 'Inter', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['ui-monospace', 'SF Mono', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      colors: {
        zinc: neutral,
        emerald: indigo,
        teal: fuchsia,
        accent: indigo,
      },
      boxShadow: {
        glow: '0 0 0 1px rgba(124,58,237,0.35), 0 10px 34px -10px rgba(124,58,237,0.50)',
      },
    },
  },
  plugins: [],
};
