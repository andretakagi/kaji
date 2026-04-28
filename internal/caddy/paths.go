package caddy

import "fmt"

const (
	httpServersPath   = "apps/http/servers"
	tlsAutomationPath = "apps/tls/automation"
	tlsPoliciesPath   = "apps/tls/automation/policies"
	loggingLogsPath   = "logging/logs"
)

func serverPath(server string) string {
	return httpServersPath + "/" + server
}

func serverCaddyRoutesPath(server string) string {
	return serverPath(server) + "/routes"
}

func serverCaddyRoutePath(server string, index int) string {
	return fmt.Sprintf("%s/routes/%d", serverPath(server), index)
}

func serverAutoHTTPSPath(server string) string {
	return serverPath(server) + "/automatic_https"
}

func serverMetricsPath(server string) string {
	return serverPath(server) + "/metrics"
}

func serverLoggerNamesPath(server string) string {
	return serverPath(server) + "/logs/logger_names"
}

func serverLoggerNamePath(server, domain string) string {
	return serverLoggerNamesPath(server) + "/" + domain
}

func serverErrorsPath(server string) string {
	return serverPath(server) + "/errors"
}

// LogSinkPath returns the Caddy config path for a named log sink.
func LogSinkPath(name string) string {
	return loggingLogsPath + "/" + name
}

func tlsPolicyIssuersPath(index int) string {
	return fmt.Sprintf("%s/%d/issuers", tlsPoliciesPath, index)
}
