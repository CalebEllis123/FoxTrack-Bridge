package main

import _ "embed"

//go:embed web/ui.html
var uiHTML []byte

//go:embed assets/logo.png
var logoPNG []byte

//go:embed web/tailwind.css
var tailwindCSS []byte

//go:embed web/icons.css
var iconsCSS []byte
