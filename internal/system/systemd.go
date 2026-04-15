// SystemdManager runs Caddy via systemctl. Used on bare metal.
package system

import (
	"errors"
	"fmt"
	"os/exec"
)

type SystemdManager struct {
	AdminURL string
}

func (s *SystemdManager) Start() error {
	if err := exec.Command("systemctl", "start", "caddy").Run(); err != nil {
		return fmt.Errorf("systemctl start caddy: %w", err)
	}
	if err := WaitForAdminAPI(adminURL(s.AdminURL), adminReadyTimeout); err != nil {
		return fmt.Errorf("caddy started but admin API not reachable: %w", err)
	}
	return nil
}

func (s *SystemdManager) Stop() error {
	if err := exec.Command("systemctl", "stop", "caddy").Run(); err != nil {
		return fmt.Errorf("systemctl stop caddy: %w", err)
	}
	return nil
}

func (s *SystemdManager) Restart() error {
	if err := exec.Command("systemctl", "restart", "caddy").Run(); err != nil {
		return fmt.Errorf("systemctl restart caddy: %w", err)
	}
	if err := WaitForAdminAPI(adminURL(s.AdminURL), adminReadyTimeout); err != nil {
		return fmt.Errorf("caddy restarted but admin API not reachable: %w", err)
	}
	return nil
}

func (s *SystemdManager) Status() (bool, error) {
	err := exec.Command("systemctl", "is-active", "--quiet", "caddy").Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, fmt.Errorf("systemctl is-active caddy: %w", err)
}
