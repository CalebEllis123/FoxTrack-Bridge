package main

// iconBytes is a 32x32 ICO file containing an orange square.
// Windows requires ICO format for systray icons.
// To use your real FoxTrack logo, replace this with an ICO converted
// from your PNG using: https://convertio.co/png-ico/ (32x32 px)
// Then run: xxd -i foxtrack.ico | grep -A999 "unsigned char"
// and paste the bytes here.
var iconBytes = []byte{
	// ICO header
	0x00, 0x00, // reserved
	0x01, 0x00, // type: 1 = ICO
	0x01, 0x00, // image count: 1
	// Image directory entry
	0x20,       // width: 32
	0x20,       // height: 32
	0x00,       // color count: 0 (>8bpp)
	0x00,       // reserved
	0x01, 0x00, // color planes
	0x20, 0x00, // bits per pixel: 32
	// Image data size (little-endian) = 40 + 32*32*4 = 40 + 4096 = 4136 = 0x1028
	0x28, 0x10, 0x00, 0x00,
	// Offset to image data = 6 (header) + 16 (dir entry) = 22 = 0x16
	0x16, 0x00, 0x00, 0x00,
	// BITMAPINFOHEADER (40 bytes)
	0x28, 0x00, 0x00, 0x00, // header size: 40
	0x20, 0x00, 0x00, 0x00, // width: 32
	0x40, 0x00, 0x00, 0x00, // height: 64 (32 * 2 for XOR+AND masks)
	0x01, 0x00,             // color planes: 1
	0x20, 0x00,             // bits per pixel: 32
	0x00, 0x00, 0x00, 0x00, // compression: none
	0x00, 0x10, 0x00, 0x00, // image size: 4096
	0x00, 0x00, 0x00, 0x00, // X pixels per meter
	0x00, 0x00, 0x00, 0x00, // Y pixels per meter
	0x00, 0x00, 0x00, 0x00, // colors in table
	0x00, 0x00, 0x00, 0x00, // important colors
}

func init() {
	// Append 32x32 pixels of orange (BGRA: B=0, G=101, R=245, A=255 = #f56500 ≈ orange-500)
	// ICO pixel data is bottom-to-top
	pixel := []byte{0x00, 0x65, 0xf5, 0xff} // BGRA orange
	for i := 0; i < 32*32; i++ {
		iconBytes = append(iconBytes, pixel...)
	}
	// AND mask: 32 rows * 4 bytes (32 bits, all 0 = opaque)
	for i := 0; i < 32*4; i++ {
		iconBytes = append(iconBytes, 0x00)
	}
}
