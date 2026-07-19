package main

import "embed"

//go:embed web/ui.html
var uiHTML []byte

//go:embed assets/logo.png
var logoPNG []byte

//go:embed web/tailwind.css
var tailwindCSS []byte

//go:embed web/icons.css
var iconsCSS []byte

//go:embed web/fonts.css
var fontsCSS []byte

//go:embed web/fonts
var fontsFS embed.FS
