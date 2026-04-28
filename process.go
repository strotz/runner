package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
)

// Process wraps an os/exec.Cmd with output capturing and keyword search capabilities.
type Process struct {
	shortName string
	cmd       *exec.Cmd
	stdout    *sniffer
	stderr    *sniffer
}

// NewProcess creates a new Process for the given executable name and arguments.
// It pipes stdout and stderr through formatted printers and sniffers for searchable output.
func NewProcess(ctx context.Context, name string, args ...string) (*Process, error) {
	_, fileName := path.Split(name)
	cmd := exec.CommandContext(ctx, name, args...)
	c := cmd.Cancel
	cmd.Cancel = func() error {
		log.Println("Cancel called for ", fileName)
		return c()
	}
	// Pipe stdout and stderr of the process to the test execution stderr.
	testOutput := &formattedPrinter{
		out:  os.Stderr,
		name: fileName,
	}
	// Shasta applications use stderr for debug and info messages. Let's add a sniffer to make it searchable.
	stdout := newSniffer(testOutput)
	cmd.Stdout = stdout
	stderr := newSniffer(testOutput)
	cmd.Stderr = stderr
	return &Process{
		shortName: fileName,
		cmd:       cmd,
		stdout:    stdout,
		stderr:    stderr,
	}, nil
}

// ChangeDirectory sets the working directory for the process.
func (p *Process) ChangeDirectory(path string) {
	p.cmd.Dir = path
}

// Start starts the process without waiting for it to exit.
func (p *Process) Start() error {
	// Log the command being executed.
	log.Printf("Executing command: %s %s", p.cmd.Path, strings.Join(p.cmd.Args, " "))
	err := p.cmd.Start()
	if err != nil {
		return err
	}
	log.Printf("process '%s' started with PID %d", p.shortName, p.cmd.Process.Pid)
	return nil
}

// StartAsync executes the process and starts processing its stderr. It signals that process exits via waitDone.
func (p *Process) StartAsync(waitDone *sync.WaitGroup) error {
	if err := p.Start(); err != nil {
		return err
	}
	waitDone.Add(1)
	go func() {
		defer waitDone.Done()
		p.RunUntilExit()
	}()
	return nil
}

// RunWithMarker starts the process and waits until a given marker string appears in the stderr.
// It is a typical way to execute applications.
func (p *Process) RunWithMarker(ctx context.Context, waitDone *sync.WaitGroup, marker string) error {
	if err := p.StartAsync(waitDone); err != nil {
		return err
	}
	err := p.StdErrScanner().WaitForKeyword(ctx, marker)
	if err != nil {
		if !errors.Is(err, KeywordNotFound) {
			p.Kill()
		}
		return err
	}
	log.Println(p.shortName, "is running as expected")
	return nil
}

// RunUntilExit waits for the process to exit and closes its stdout and stderr sniffers.
// It logs a fatal error if the process exits with an unexpected error.
func (p *Process) RunUntilExit() {
	err := p.cmd.Wait()
	pid := p.cmd.Process.Pid
	if err1 := p.stdout.Close(); err1 != nil {
		log.Println("Error closing stdout:", err1, "of process", p.shortName, pid)
	}
	if err1 := p.stderr.Close(); err1 != nil {
		log.Println("Error closing stderr:", err1, "of process", p.shortName, pid)
	}
	if err != nil {
		log.Println(p.shortName, "error:", err, p.cmd.Process.Pid, "exit state:", p.cmd.ProcessState.String())
		if errors.Is(err, context.Canceled) {
			return
		}
		if strings.Contains(err.Error(), "signal: killed") {
			return
		}
		if strings.Contains(err.Error(), "signal: interrupt") {
			return
		}
		log.Fatalln("Process exited with error:", err)
	}
}

// Kill forcefully terminates the process.
func (p *Process) Kill() {
	log.Println("Killing process:", p.shortName, p.cmd.Process.Pid)
	if err := p.cmd.Process.Kill(); err != nil {
		log.Fatal(err)
	}
}

// Interrupt sends an interrupt signal (Ctrl-C / SIGINT) to the process.
// This allows for graceful shutdown testing by simulating a user pressing Ctrl-C.
func (p *Process) Interrupt() {
	log.Printf("Interrupting process: %s (PID %d)", p.shortName, p.cmd.Process.Pid)
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		log.Fatal(err)
	}
}

// AddEnv appends an environment variable to the process in NAME=value format.
func (p *Process) AddEnv(name string, value string) {
	p.cmd.Env = append(p.cmd.Env, fmt.Sprintf("%s=%s", name, value))
}

// IsAlive returns true if the process has been started and has not yet exited.
func (p *Process) IsAlive() bool {
	// ProcessState populated when process exits.
	return p.cmd != nil && p.cmd.ProcessState == nil
}

// NewStdOutReader returns a new reader for the process stdout buffer.
func (p *Process) NewStdOutReader() io.ReadCloser {
	return p.stdout.NewReader()
}

// NewStdErrReader returns a new reader for the process stderr buffer.
func (p *Process) NewStdErrReader() io.ReadCloser {
	return p.stderr.NewReader()
}

// StdOutScanner returns the main BufferSearch for stdout, continuing from the last search position.
func (p *Process) StdOutScanner() BufferSearch {
	return p.stdout
}

// StdErrScanner returns the main BufferSearch for stderr, continuing from the last search position.
func (p *Process) StdErrScanner() BufferSearch {
	return p.stderr
}

// NewStdOutSearch returns a new BufferSearch for stdout. It starts searching from the beginning.
func (p *Process) NewStdOutSearch() BufferSearch {
	return p.stdout.NewSearch()
}

// NewStdErrSearch returns a new BufferSearch for stderr. It starts searching from the beginning.
func (p *Process) NewStdErrSearch() BufferSearch {
	return p.stderr.NewSearch()
}

func readLines(in io.Reader) ([]string, error) {
	s := bufio.NewScanner(in)
	lines := []string{}
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	return lines, s.Err()
}

// ReadStdOut reads all stdout output and returns it as a slice of lines.
func (p *Process) ReadStdOut() ([]string, error) {
	return readLines(p.NewStdOutReader())
}

// ReadStdErr reads all stderr output and returns it as a slice of lines.
func (p *Process) ReadStdErr() ([]string, error) {
	return readLines(p.NewStdErrReader())
}
