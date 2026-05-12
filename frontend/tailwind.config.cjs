/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Yodel Grotesk"', "Arial", "sans-serif"],
        display: ['"Yodel Grotesk"', "Arial", "sans-serif"],
        news: ['"Yodel Grotesk"', "Arial", "sans-serif"],
        mono: [
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Monaco",
          "Consolas",
          '"Liberation Mono"',
          '"Courier New"',
          "monospace",
        ],
      },
      colors: {
        panel: {
          900: "#000000",
          800: "#080808",
          700: "#16181c",
          650: "#111418",
          600: "#0f1419",
          500: "#2f3336",
          400: "#71767b",
          100: "#e7e9ea",
        },
        brand: {
          500: "#1d9bf0",
          400: "#4fb3ff",
        },
      },
      boxShadow: {
        panel: "0 14px 40px rgba(0, 0, 0, 0.42)",
      },
    },
  },
  plugins: [],
};
