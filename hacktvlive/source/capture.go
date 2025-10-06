package source

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"runtime"

	"hacktvlive/config"
	"hacktvlive/video"
)

// StartFFmpegCapture starts an FFmpeg process to capture video.
func StartFFmpegCapture(cfg *config.Config, v video.Standard) (*exec.Cmd, error) {
	var ffmpegArgs []string

	switch runtime.GOOS {
	case "linux":
		dev := cfg.Device
		if dev == "" {
			dev = "/dev/video0"
		}
		ffmpegArgs = []string{"-f", "v4l2", "-i", dev}
	case "darwin":
		dev := cfg.Device
		if dev == "" {
			dev = "0"
		}
		ffmpegArgs = []string{"-f", "avfoundation", "-i", dev}
	case "windows":
		dev := cfg.Device
		if dev == "" {
			dev = "Integrated Webcam"
		}
		ffmpegArgs = []string{"-f", "dshow", "-i", "video=" + dev}
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	fpsVal := "30000/1001"
	if cfg.PAL {
		fpsVal = "25"
	}

	var vfArg string
	if cfg.Callsign != "" {
		vfArg = fmt.Sprintf("scale=%d:%d,fps=%s,drawbox=x=0:y=ih-40:w=iw:h=40:color=black@0.6:t=fill,drawtext=fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf:text='%s':x=10:y=h-35:fontcolor=white:fontsize=32:borderw=2:bordercolor=black", video.FrameWidth, video.FrameHeight, fpsVal, cfg.Callsign)
	} else {
		vfArg = fmt.Sprintf("scale=%d:%d,fps=%s", video.FrameWidth, video.FrameHeight, fpsVal)
	}

	commonArgs := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "nobuffer", "-flags", "low_delay",
		"-probesize", "32", "-analyzeduration", "0",
		"-threads", "1", "-f", "rawvideo",
		"-pix_fmt", "rgb24", "-vf", vfArg, "-",
	}

	ffmpegArgs = append(ffmpegArgs, commonArgs...)
	ffmpegCmd := exec.Command("ffmpeg", ffmpegArgs...)

	ffmpegStdout, err := ffmpegCmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get FFmpeg stdout pipe: %w", err)
	}
	if err := ffmpegCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start FFmpeg: %w", err)
	}
	log.Println("FFmpeg process started to capture webcam...")

	go func() {
		for {
			// Lock the raw buffer before writing to prevent a data race.
			v.LockRaw()
			_, err := io.ReadFull(ffmpegStdout, v.RawFrameBuffer())
			v.UnlockRaw() // Always unlock, even after an error.

			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading from FFmpeg: %v", err)
				}
				break
			}

			// Generate the full analog signal frame from the new raw data.
			v.LockFrame()
			v.GenerateFullFrame()
			v.UnlockFrame()
		}
	}()

	return ffmpegCmd, nil
}