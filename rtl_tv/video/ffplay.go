package video

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"rtltv/config" // Import our new config package
)

// FFplay represents the FFplay video player process and its input pipe.
type FFplay struct {
	Pipe io.WriteCloser
	Cmd  *exec.Cmd
}

// Start launches the FFplay process configured for our raw video stream.
func Start() (*FFplay, error) {
	ffplayPath, err := exec.LookPath("ffplay")
	if err != nil {
		return nil, fmt.Errorf("ffplay not found in your PATH")
	}

	args := []string{
		"-f", "rawvideo",
		"-pixel_format", "rgb24",
		"-video_size", fmt.Sprintf("%dx%d", config.FrameWidth, config.FrameHeight),
		"-framerate", fmt.Sprintf("%f", config.FrameRate),
		"-i", "-", // Read from stdin
		"-window_title", "NTSC Receiver",
		"-x", "720", "-y", "480",
		"-fflags", "nobuffer",
		"-flags", "low_delay",
	}

	cmd := exec.Command(ffplayPath, args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr // Show ffplay errors in our console

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	log.Println("FFplay process started. Video output should appear in a new window.")
	return &FFplay{Pipe: stdinPipe, Cmd: cmd}, nil
}

// Stop safely terminates the FFplay process.
func (f *FFplay) Stop() {
	f.Pipe.Close()
	f.Cmd.Process.Kill()
}