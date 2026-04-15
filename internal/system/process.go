// ProcessManager spawns Caddy as a child process. Used in Docker
// and anywhere systemd isn't available.
package system

import (
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	crashDetectDelay = 500 * time.Millisecond
	stopGraceTimeout = 5 * time.Second
)

type ProcessManager struct {
	AdminURL string
	mu       sync.Mutex
	cmd      *exec.Cmd
	done     chan struct{}
	stopping bool
	exitErr  error
}

func (p *ProcessManager) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return fmt.Errorf("caddy already running")
	}

	cmd := exec.Command("caddy", "run")

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting caddy process: %w", err)
	}
	p.cmd = cmd
	p.done = make(chan struct{})
	p.stopping = false
	p.exitErr = nil

	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.cmd = nil
		p.exitErr = err
		p.mu.Unlock()
		close(p.done)
	}()

	// Wait briefly to catch immediate crashes (bad config, missing binary, etc.)
	select {
	case <-p.done:
		p.mu.Lock()
		err := p.exitErr
		p.mu.Unlock()
		if err != nil {
			return fmt.Errorf("caddy exited immediately: %w", err)
		}
		return fmt.Errorf("caddy exited immediately")
	case <-time.After(crashDetectDelay):
	}

	// Wait for the admin API to become reachable
	if err := p.waitForAdminAPI(); err != nil {
		return fmt.Errorf("caddy started but admin API not reachable: %w", err)
	}

	return nil
}

func (p *ProcessManager) waitForAdminAPI() error {
	client := &http.Client{Timeout: adminPollTimeout}
	deadline := time.Now().Add(adminReadyTimeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(adminURL(p.AdminURL) + "/config/")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		// Check if caddy died while we were waiting
		select {
		case <-p.done:
			p.mu.Lock()
			err := p.exitErr
			p.mu.Unlock()
			if err != nil {
				return fmt.Errorf("caddy exited: %w", err)
			}
			return fmt.Errorf("caddy exited unexpectedly")
		case <-time.After(adminPollInterval):
		}
	}
	return fmt.Errorf("timed out after %s", adminReadyTimeout)
}

func (p *ProcessManager) Stop() error {
	p.mu.Lock()
	if p.cmd == nil {
		p.mu.Unlock()
		return nil
	}
	if p.stopping {
		done := p.done
		p.mu.Unlock()
		<-done
		return nil
	}
	p.stopping = true
	pgid := p.cmd.Process.Pid
	done := p.done
	p.mu.Unlock()

	syscall.Kill(-pgid, syscall.SIGTERM)

	select {
	case <-done:
		return nil
	case <-time.After(stopGraceTimeout):
		p.mu.Lock()
		if p.cmd != nil {
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
		p.mu.Unlock()
		<-done
		return nil
	}
}

func (p *ProcessManager) Restart() error {
	if err := p.Stop(); err != nil {
		return fmt.Errorf("stopping caddy for restart: %w", err)
	}
	return p.Start()
}

func (p *ProcessManager) Status() (bool, error) {
	p.mu.Lock()
	managed := p.cmd != nil
	p.mu.Unlock()
	if managed {
		return true, nil
	}
	// No managed process, but Caddy may already be running (e.g. after
	// a syscall.Exec restart where we lost the child reference). Check
	// the admin API to find out.
	client := &http.Client{Timeout: adminPollTimeout}
	resp, err := client.Get(adminURL(p.AdminURL) + "/config/")
	if err != nil {
		return false, nil
	}
	resp.Body.Close()
	return true, nil
}
