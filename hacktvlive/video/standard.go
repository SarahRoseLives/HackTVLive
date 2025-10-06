package video

// Video source resolution we will ask FFmpeg to produce
const (
	FrameWidth  = 540
	FrameHeight = 480
)

// Standard defines the interface for a video signal standard like NTSC or PAL.
type Standard interface {
	GenerateFullFrame()
	FillTestPattern()
	IreToAmplitude(float64) float64
	// Mutex for the final, generated frame (NTSC/PAL signal)
	LockFrame()
	UnlockFrame()
	RLockFrame()
	RUnlockFrame()
	// Mutex for the raw RGB frame from FFmpeg
	LockRaw()
	UnlockRaw()
	// Buffer accessors
	FrameBuffer() []float64
	RawFrameBuffer() []byte
}