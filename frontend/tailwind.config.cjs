/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Yodel Grotesk"', "Arial", "sans-serif"],
        display: ['"Yodel Grotesk"', "Arial", "sans-serif"],
        news: ['"Yodel Grotesk"', "Arial", "sans-serif"],
        mono: ['"Yodel Grotesk"', "Arial", "sans-serif"],
      },
      colors: {
        panel: {
          900: "#0b0f14",
          800: "#151b24",
          700: "#1b2330",
          650: "#101722",
          600: "#0f151f",
          500: "#2a3443",
          400: "#9aabbe",
          100: "#e8edf5",
        },
        brand: {
          500: "#4f8cff",
          400: "#5f98ff",
        },
      },
      boxShadow: {
        panel: "0 14px 40px rgba(0, 0, 0, 0.35)",
      },
    },
  },
  plugins: [],
};
