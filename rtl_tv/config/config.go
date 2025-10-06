package config

import "flag"

// Video constants define the NTSC frame structure.
const (
	FrameWidth    = 540
	FrameHeight   = 480
	FrameRate     = 30000.0 / 1001.0
	LinesPerFrame = 525
)

// Timing constants for the NTSC signal in microseconds.
const (
	HsyncDurationMicroseconds = 4.7
	FrontPorchMicroseconds    = 1.5
	ActiveVideoMicroseconds   = 52.6
)

// SDRConfig holds settings for the RTL-SDR device.
type SDRConfig struct {
	FrequencyHz  int
	SampleRateHz int
	Gain         int
}

// AppConfig holds the application's entire configuration.
type AppConfig struct {
	SDR SDRConfig
}

// ParseFlags parses command-line flags and returns an AppConfig.
func ParseFlags() *AppConfig {
	bw := flag.Float64("bw", 1.5, "SDR sample rate (bandwidth) in MHz")
	freq := flag.Float64("freq", 1280, "SDR center frequency in MHz")
	gain := flag.Int("gain", 496, "SDR tuner gain in tenths of a dB (e.g., 496 for 49.6 dB)")
	flag.Parse()

	return &AppConfig{
		SDR: SDRConfig{
			FrequencyHz:  int(*freq * 1_000_000),
			SampleRateHz: int(*bw * 1_000_000),
			Gain:         *gain,
		},
	}
}