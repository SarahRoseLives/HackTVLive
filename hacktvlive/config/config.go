package config

import "flag"

// FixedSampleRate is the constant sample rate for the HackRF, set to 8 Msps.
const FixedSampleRate = 8_000_000.0

// Config holds all application configuration values.
type Config struct {
	Frequency float64
	Bandwidth float64
	Gain      int
	Device    string
	Callsign  string
	Test      bool
	PAL       bool
}

// New creates and returns a new Config struct populated from command-line flags.
func New() *Config {
	cfg := &Config{}
	flag.Float64Var(&cfg.Frequency, "freq", 1280, "Transmit frequency in MHz")
	flag.Float64Var(&cfg.Bandwidth, "bw", 1.5, "Channel bandwidth in MHz for filtering")
	flag.IntVar(&cfg.Gain, "gain", 30, "TX VGA gain (0-47)")
	flag.StringVar(&cfg.Device, "device", "", "Video device name or index (OS-dependent)")
	flag.StringVar(&cfg.Callsign, "callsign", "NOCALL", "Callsign to overlay on the video")
	flag.BoolVar(&cfg.Test, "test", false, "Show SMPTE colorbar test screen instead of webcam")
	flag.BoolVar(&cfg.PAL, "pal", false, "Use PAL standard instead of NTSC")
	flag.Parse()

	return cfg
}