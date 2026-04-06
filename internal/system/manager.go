// CaddyManager interface and factory. Picks SystemdManager or ProcessManager
// depending on the environment.
package system

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type CaddyManager interface {
	Start() error
	Stop() error
	Restart() error
	Status() (bool, error)
}

func NewCaddyManager(adminURL string) CaddyManager {
	if os.Getenv("CADDY_GUI_MODE") == "docker" {
		return &ProcessManager{AdminURL: adminURL}
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		return &SystemdManager{AdminURL: adminURL}
	}
	return &ProcessManager{AdminURL: adminURL}
}

// Polls until the admin API responds or timeout. Called after starting
// Caddy so we don't try to load config before it's ready.
func WaitForAdminAPI(adminURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(adminURL + "/config/")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("caddy admin API not reachable after %s", timeout)
}
