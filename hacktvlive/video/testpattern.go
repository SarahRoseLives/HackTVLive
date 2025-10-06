package video

// FillColorBars fills the rawFrameBuffer with a standard SMPTE color bars pattern.
func FillColorBars(buf []byte) {
	// SMPTE color bars: 7 vertical stripes
	barColors := [7][3]uint8{
		{192, 192, 192}, // Gray
		{192, 192, 0},   // Yellow
		{0, 192, 192},   // Cyan
		{0, 192, 0},     // Green
		{192, 0, 192},   // Magenta
		{192, 0, 0},     // Red
		{0, 0, 192},     // Blue
	}
	barWidth := FrameWidth / 7
	for y := 0; y < FrameHeight; y++ {
		for x := 0; x < FrameWidth; x++ {
			barIdx := x / barWidth
			if barIdx >= 7 {
				barIdx = 6
			}
			i := (y*FrameWidth + x) * 3
			buf[i] = barColors[barIdx][0]
			buf[i+1] = barColors[barIdx][1]
			buf[i+2] = barColors[barIdx][2]
		}
	}
}