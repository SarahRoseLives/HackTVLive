package config

import "flag"

// Config holds all application configuration values.
type Config struct {
	Frequency  float64
	Bandwidth  float64
	SampleRate float64
	Gain       int
	Device     string
	Callsign   string
	Test       bool
	PAL        bool
}

// New creates and returns a new Config struct populated from command-line flags.
func New() *Config {
	cfg := &Config{}
	flag.Float64Var(&cfg.Frequency, "freq", 1280, "Transmit frequency in MHz")
	flag.Float64Var(&cfg.Bandwidth, "bw", 1.5, "Channel bandwidth in MHz")
	flag.IntVar(&cfg.Gain, "gain", 30, "TX VGA gain (0-47)")
	flag.StringVar(&cfg.Device, "device", "", "Video device name or index (OS-dependent)")
	flag.StringVar(&cfg.Callsign, "callsign", "NOCALL", "Callsign to overlay on the video")
	flag.BoolVar(&cfg.Test, "test", false, "Show SMPTE colorbar test screen instead of webcam")
	flag.BoolVar(&cfg.PAL, "pal", false, "Use PAL standard instead of NTSC")
	flag.Parse()

	// Calculate sample rate from bandwidth
	cfg.SampleRate = cfg.Bandwidth * 1_000_000

	return cfg
}