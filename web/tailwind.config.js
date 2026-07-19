/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./web/ui.html'],
  theme: {
    extend: {
      colors: {
        zinc: {
          950: '#09090b', 900: '#101012', 800: '#17171a',
          700: '#1f2023', 600: '#2c2d31', 500: '#3e4046',
          400: '#5b5d63', 300: '#7a7c82', 200: '#9a9ca2',
          100: '#bcbec4', 50: '#e3e4e6'
        }
      }
    }
  }
}
