package main

import _ "embed"

//go:embed web/ui.html
var uiHTML []byte

//go:embed assets/logo.png
var logoPNG []byte
