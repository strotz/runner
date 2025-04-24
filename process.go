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

type Process struct {
	shortName string
	cmd       *exec.Cmd
	stdout    *AccumulatedOutput
	stderr    *AccumulatedOutput
}

func NewProcess(ctx context.Context, name string, args ...string) (*Process, error) {
	_, fileName := path.Split(name)
	cmd := exec.CommandContext(ctx, name, args...)
	c := cmd.Cancel
	cmd.Cancel = func() error {
		log.Println("Cancel called for ", fileName)
		return c()
	}
	// Pipe stdout and stderr of the process to the test execution stderr.
	testOutput := &FormattedPrinter{
		Out:    os.Stderr,
		Prefix: fileName,
	}
	stdout := NewAccumulatedOutput(testOutput)
	cmd.Stdout = stdout
	stderr := NewAccumulatedOutput(testOutput)
	cmd.Stderr = stderr
	return &Process{
		shortName: fileName,
		cmd:       cmd,
		stdout:    stdout,
		stderr:    stderr,
	}, nil
}

func (p *Process) ChangeDirectory(path string) {
	p.cmd.Dir = path
}

func (p *Process) Start() error {
	err := p.cmd.Start()
	if err != nil {
		return err
	}
	log.Printf("process '%s' started", p.shortName)
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

func (p *Process) RunUntilExit() {
	err := p.cmd.Wait()
	// TODO: it is not clear if we should close the output streams here.
	_ = p.stdout.Close()
	_ = p.stderr.Close()
	if err != nil {
		log.Println(p.shortName, "error:", err, p.cmd.Process.Pid)
		if errors.Is(err, context.Canceled) {
			return
		}
		if strings.Contains(err.Error(), "signal: killed") {
			return
		}
		log.Fatalln("Process exited with error:", err)
	}
}

func (p *Process) Kill() {
	log.Println("Killing process:", p.shortName, p.cmd.Process.Pid)
	if err := p.cmd.Process.Kill(); err != nil {
		log.Fatal(err)
	}
}

func (p *Process) AddEnv(name string, value string) {
	p.cmd.Env = append(p.cmd.Env, fmt.Sprintf("%s=%s", name, value))
}

func (p *Process) IsAlive() bool {
	// ProcessState populated when process exits.
	return p.cmd != nil && p.cmd.ProcessState == nil
}

func (p *Process) NewStdOutReader() io.ReadCloser {
	return p.stdout.NewReader()
}

func (p *Process) NewStdErrReader() io.ReadCloser {
	return p.stderr.NewReader()
}

func readLines(in io.Reader) ([]string, error) {
	s := bufio.NewScanner(in)
	lines := []string{}
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	return lines, s.Err()
}

func (p *Process) ReadStdOut() ([]string, error) {
	return readLines(p.NewStdOutReader())
}

func (p *Process) ReadStdErr() ([]string, error) {
	return readLines(p.NewStdErrReader())
}

type OutputScanner interface {
	// WaitForKeyword scans the output stream for given substr. It is a blocking call.
	WaitForKeyword(ctx context.Context, substr string) error
}

func (p *Process) StdOutScanner() OutputScanner {
	return p.stdout
}

func (p *Process) StdErrScanner() OutputScanner {
	return p.stderr
}
