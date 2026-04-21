// Caddyfile generation and parsing. Converts between Caddy JSON and Caddyfile text.
package caddy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type CaddyfileSettings struct {
	ACMEEmail   string        `json:"acme_email"`
	AdminListen string        `json:"admin_listen"`
	Toggles     GlobalToggles `json:"global_toggles"`
	DomainCount int           `json:"domain_count"`
}

func ExtractCaddyfileSettings(adaptedJSON json.RawMessage) (*CaddyfileSettings, error) {
	cfg, err := parseCaddyfileConfig(adaptedJSON, "")
	if err != nil {
		return nil, fmt.Errorf("parsing adapted caddyfile config: %w", err)
	}

	var top struct {
		Admin *struct {
			Listen string `json:"listen"`
		} `json:"admin"`
	}
	if err := json.Unmarshal(adaptedJSON, &top); err != nil {
		return nil, fmt.Errorf("parsing admin config: %w", err)
	}

	toggles := GlobalToggles{
		AutoHTTPS: cfg.AutoHTTPS,
	}
	if toggles.AutoHTTPS == "" {
		toggles.AutoHTTPS = "on"
	}
	if cfg.Metrics {
		toggles.PrometheusMetrics = true
		toggles.PerHostMetrics = cfg.PerHostMetrics
	}

	domainCount := 0
	for _, srv := range cfg.Servers {
		domainCount += len(srv.Domains)
	}

	adminListen := ""
	if top.Admin != nil && top.Admin.Listen != "" {
		adminListen = top.Admin.Listen
	}

	return &CaddyfileSettings{
		ACMEEmail:   cfg.ACMEEmail,
		AdminListen: adminListen,
		Toggles:     toggles,
		DomainCount: domainCount,
	}, nil
}

// ParseCaddyfileAdminAddr extracts the admin listen address from raw Caddyfile
// text. Caddy's /adapt endpoint often omits the admin block from its output,
// so this parses the source text directly as a fallback.
func ParseCaddyfileAdminAddr(caddyfileText string) string {
	lines := strings.Split(caddyfileText, "\n")
	inGlobal := false
	depth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if !inGlobal {
			if trimmed == "{" {
				inGlobal = true
				depth = 1
				continue
			}
			return ""
		}

		if depth == 1 {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 && fields[0] == "admin" && fields[1] != "off" && fields[1] != "{" {
				return fields[1]
			}
		}

		for _, ch := range trimmed {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
			}
		}

		if depth <= 0 {
			return ""
		}
	}

	return ""
}

type caddyfileLogWriter struct {
	Output      string  `json:"output"`
	Filename    string  `json:"filename"`
	RollSizeMB  int     `json:"roll_size_mb"`
	RollKeep    int     `json:"roll_keep"`
	RollKeepFor float64 `json:"roll_keep_for"` // nanoseconds
}

type caddyfileLogEncoder struct {
	Format string `json:"format"`
}

type caddyfileLogger struct {
	Writer  *caddyfileLogWriter  `json:"writer"`
	Encoder *caddyfileLogEncoder `json:"encoder"`
	Level   string               `json:"level"`
	Include []string             `json:"include"`
	Exclude []string             `json:"exclude"`
}

// caddyfileConfig is the subset of Caddy's JSON config we need for Caddyfile generation.
type caddyfileConfig struct {
	ACMEEmail      string
	AutoHTTPS      string // "on" (default/omit), "off", "disable_redirects"
	Metrics        bool
	PerHostMetrics bool
	Servers        map[string]caddyfileServer
	LogFile        string // kaji_access logger file path
	Loggers        map[string]caddyfileLogger
}

type caddyfileServer struct {
	Domains    []DomainParams
	LogDomains map[string]string // domain -> sink name
}

func parseCaddyfileConfig(raw json.RawMessage, fallbackLogFile string) (*caddyfileConfig, error) {
	var full struct {
		Apps struct {
			HTTP struct {
				Servers map[string]caddyServer `json:"servers"`
			} `json:"http"`
			TLS struct {
				Automation struct {
					Policies []tlsPolicy `json:"policies"`
				} `json:"automation"`
			} `json:"tls"`
		} `json:"apps"`
		Logging *struct {
			Logs map[string]caddyfileLogger `json:"logs"`
		} `json:"logging"`
	}
	if err := json.Unmarshal(raw, &full); err != nil {
		return nil, fmt.Errorf("parsing caddy config for caddyfile: %w", err)
	}

	cfg := &caddyfileConfig{
		AutoHTTPS: "on",
		LogFile:   fallbackLogFile,
		Servers:   make(map[string]caddyfileServer),
		Loggers:   make(map[string]caddyfileLogger),
	}

	cfg.ACMEEmail = acmeEmailFromPolicies(full.Apps.TLS.Automation.Policies)

	if full.Logging != nil {
		for name, logger := range full.Logging.Logs {
			cfg.Loggers[name] = logger
		}
		if kajiLog, ok := cfg.Loggers["kaji_access"]; ok && kajiLog.Writer != nil && kajiLog.Writer.Filename != "" {
			cfg.LogFile = kajiLog.Writer.Filename
		}
	}

	// Auto HTTPS and metrics from first server
	for _, srv := range full.Apps.HTTP.Servers {
		if srv.AutoHTTPS != nil {
			if srv.AutoHTTPS.Disable {
				cfg.AutoHTTPS = "off"
			} else if srv.AutoHTTPS.DisableRedirects {
				cfg.AutoHTTPS = "disable_redirects"
			}
		}
		if srv.Metrics != nil {
			cfg.Metrics = true
			cfg.PerHostMetrics = srv.Metrics.PerHost
		}
		break
	}

	// Domains and access log mappings per server
	for name, srv := range full.Apps.HTTP.Servers {
		cs := caddyfileServer{
			LogDomains: make(map[string]string),
		}
		if srv.Logs != nil {
			for domain, logger := range srv.Logs.LoggerNames {
				cs.LogDomains[domain] = logger
			}
		}
		for _, raw := range srv.Routes {
			params, err := ParseDomainParams(raw)
			if err != nil || params.Domain == "" {
				continue
			}
			params.Toggles.AccessLog = cs.LogDomains[params.Domain]
			cs.Domains = append(cs.Domains, params)
		}
		cfg.Servers[name] = cs
	}

	return cfg, nil
}

func GenerateCaddyfile(raw json.RawMessage, logFile string) (string, error) {
	cfg, err := parseCaddyfileConfig(raw, logFile)
	if err != nil {
		return "", fmt.Errorf("parsing config for caddyfile generation: %w", err)
	}

	var b strings.Builder

	writeGlobalOptions(&b, cfg)

	serverNames := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	var logWriter *caddyfileLogWriter
	if kajiLog, ok := cfg.Loggers["kaji_access"]; ok && kajiLog.Writer != nil {
		logWriter = kajiLog.Writer
	} else if cfg.LogFile != "" {
		logWriter = &caddyfileLogWriter{Output: "file", Filename: cfg.LogFile}
	}

	for _, name := range serverNames {
		srv := cfg.Servers[name]
		for _, route := range srv.Domains {
			writeSiteBlock(&b, route, logWriter)
		}
	}

	return b.String(), nil
}

func writeGlobalOptions(b *strings.Builder, cfg *caddyfileConfig) {
	var lines []string

	if cfg.ACMEEmail != "" {
		lines = append(lines, "email "+cfg.ACMEEmail)
	}
	if def, ok := cfg.Loggers["default"]; ok && def.Level == "DEBUG" && !loggerHasExtras(def) {
		lines = append(lines, "debug")
	}
	if cfg.AutoHTTPS == "off" {
		lines = append(lines, "auto_https off")
	} else if cfg.AutoHTTPS == "disable_redirects" {
		lines = append(lines, "auto_https disable_redirects")
	}

	if cfg.Metrics {
		if cfg.PerHostMetrics {
			lines = append(lines, "metrics {\n\t\tper_host\n\t}")
		} else {
			lines = append(lines, "metrics")
		}
	}

	hasLoggers := hasNonTrivialLoggers(cfg)

	if len(lines) == 0 && !hasLoggers {
		return
	}

	b.WriteString("{\n")
	for _, line := range lines {
		b.WriteString("\t" + line + "\n")
	}
	if hasLoggers {
		writeLogBlocks(b, cfg)
	}
	b.WriteString("}\n\n")
}

func loggerHasExtras(l caddyfileLogger) bool {
	return l.Writer != nil || l.Encoder != nil ||
		len(l.Include) > 0 || len(l.Exclude) > 0
}

func hasNonTrivialLoggers(cfg *caddyfileConfig) bool {
	for name, logger := range cfg.Loggers {
		if name == "kaji_access" {
			continue
		}
		if name == "default" && logger.Level == "DEBUG" && !loggerHasExtras(logger) {
			continue
		}
		if logger.Level != "" || loggerHasExtras(logger) {
			return true
		}
	}
	return false
}

func writeLogBlocks(b *strings.Builder, cfg *caddyfileConfig) {
	names := make([]string, 0, len(cfg.Loggers))
	for name := range cfg.Loggers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		logger := cfg.Loggers[name]

		// kaji_access is represented by per-site log blocks in site definitions
		if name == "kaji_access" {
			continue
		}

		// Default logger with only DEBUG level is handled by the "debug" shorthand
		if name == "default" && logger.Level == "DEBUG" &&
			logger.Writer == nil && logger.Encoder == nil &&
			len(logger.Include) == 0 && len(logger.Exclude) == 0 {
			continue
		}

		hasContent := logger.Writer != nil || logger.Encoder != nil ||
			logger.Level != "" || len(logger.Include) > 0 || len(logger.Exclude) > 0
		if !hasContent {
			continue
		}

		if name == "default" {
			b.WriteString("\tlog {\n")
		} else {
			b.WriteString("\tlog " + name + " {\n")
		}

		if logger.Writer != nil {
			writeLogWriter(b, logger.Writer)
		}
		if logger.Encoder != nil && logger.Encoder.Format != "" {
			b.WriteString("\t\tformat " + logger.Encoder.Format + "\n")
		}
		if logger.Level != "" {
			b.WriteString("\t\tlevel " + logger.Level + "\n")
		}
		for _, inc := range logger.Include {
			b.WriteString("\t\tinclude " + inc + "\n")
		}
		for _, exc := range logger.Exclude {
			b.WriteString("\t\texclude " + exc + "\n")
		}

		b.WriteString("\t}\n")
	}
}

func writeLogWriter(b *strings.Builder, w *caddyfileLogWriter) {
	switch w.Output {
	case "file":
		hasRollSettings := w.RollSizeMB > 0 || w.RollKeep > 0 || w.RollKeepFor > 0
		if hasRollSettings {
			b.WriteString("\t\toutput file " + w.Filename + " {\n")
			if w.RollSizeMB > 0 {
				b.WriteString(fmt.Sprintf("\t\t\troll_size %dMiB\n", w.RollSizeMB))
			}
			if w.RollKeep > 0 {
				b.WriteString(fmt.Sprintf("\t\t\troll_keep %d\n", w.RollKeep))
			}
			if w.RollKeepFor > 0 {
				hours := int(w.RollKeepFor / 1e9 / 3600)
				if hours > 0 {
					b.WriteString(fmt.Sprintf("\t\t\troll_keep_for %dh\n", hours))
				}
			}
			b.WriteString("\t\t}\n")
		} else {
			b.WriteString("\t\toutput file " + w.Filename + "\n")
		}
	case "stdout":
		b.WriteString("\t\toutput stdout\n")
	case "stderr":
		b.WriteString("\t\toutput stderr\n")
	default:
		if w.Output != "" {
			b.WriteString("\t\toutput " + w.Output + "\n")
		}
	}
}

func writeSiteBlock(b *strings.Builder, p DomainParams, logWriter *caddyfileLogWriter) {
	if p.Toggles.ForceHTTPS {
		b.WriteString("http://" + p.Domain + " {\n")
		b.WriteString("\tredir https://{host}{uri} 301\n")
		b.WriteString("}\n\n")
	}

	b.WriteString(p.Domain + " {\n")

	if p.Toggles.Compression {
		b.WriteString("\tencode gzip zstd\n")
	}

	if p.Toggles.Headers.Response.Enabled && p.Toggles.Headers.Response.Security {
		b.WriteString("\theader {\n")
		b.WriteString("\t\tStrict-Transport-Security \"max-age=31536000; includeSubDomains; preload\"\n")
		b.WriteString("\t\tX-Content-Type-Options \"nosniff\"\n")
		b.WriteString("\t\tX-Frame-Options \"DENY\"\n")
		b.WriteString("\t\tReferrer-Policy \"strict-origin-when-cross-origin\"\n")
		b.WriteString("\t\tPermissions-Policy \"accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()\"\n")
		b.WriteString("\t}\n")
	}

	if p.Toggles.Headers.Response.Enabled && p.Toggles.Headers.Response.CacheControl {
		b.WriteString("\theader Cache-Control \"no-store\"\n")
	}

	if p.Toggles.Headers.Response.Enabled && p.Toggles.Headers.Response.XRobotsTag {
		b.WriteString("\theader X-Robots-Tag \"noindex, nofollow\"\n")
	}

	if p.Toggles.Headers.Response.Enabled && p.Toggles.Headers.Response.CORS {
		corsOrigins := p.Toggles.Headers.Response.CORSOrigins
		if len(corsOrigins) <= 1 {
			origin := "*"
			if len(corsOrigins) == 1 {
				origin = corsOrigins[0]
			}
			b.WriteString("\theader {\n")
			b.WriteString("\t\tAccess-Control-Allow-Origin \"" + origin + "\"\n")
			b.WriteString("\t\tAccess-Control-Allow-Methods \"GET, POST, PUT, DELETE, OPTIONS\"\n")
			b.WriteString("\t\tAccess-Control-Allow-Headers \"Content-Type, Authorization\"\n")
			b.WriteString("\t}\n")
		} else {
			for i, o := range corsOrigins {
				name := fmt.Sprintf("cors%d", i)
				b.WriteString("\t@" + name + " header Origin " + o + "\n")
				b.WriteString("\theader @" + name + " Access-Control-Allow-Origin \"" + o + "\"\n")
				b.WriteString("\theader @" + name + " Access-Control-Allow-Methods \"GET, POST, PUT, DELETE, OPTIONS\"\n")
				b.WriteString("\theader @" + name + " Access-Control-Allow-Headers \"Content-Type, Authorization\"\n")
				b.WriteString("\theader @" + name + " Vary \"Origin\"\n")
			}
		}
	}

	if p.Toggles.BasicAuth.Enabled && p.Toggles.BasicAuth.Username != "" {
		b.WriteString("\tbasic_auth {\n")
		b.WriteString("\t\t" + p.Toggles.BasicAuth.Username + " " + p.Toggles.BasicAuth.PasswordHash + "\n")
		b.WriteString("\t}\n")
	}

	lbStrategy := p.Toggles.LoadBalancing.Strategy
	if p.Toggles.LoadBalancing.Enabled && lbStrategy == "" {
		lbStrategy = "round_robin"
	}

	hasRequestHeaders := p.Toggles.RequestHeaders.Enabled &&
		((p.Toggles.RequestHeaders.HostOverride && p.Toggles.RequestHeaders.HostValue != "") ||
			(p.Toggles.RequestHeaders.Authorization && p.Toggles.RequestHeaders.AuthValue != ""))
	needsBlock := p.Toggles.TLSSkipVerify || p.Toggles.WebSocketPassthru ||
		p.Toggles.LoadBalancing.Enabled || hasRequestHeaders

	if needsBlock {
		allUpstreams := p.Upstream
		if p.Toggles.LoadBalancing.Enabled {
			for _, u := range p.Toggles.LoadBalancing.Upstreams {
				allUpstreams += " " + u
			}
		}
		b.WriteString("\treverse_proxy " + allUpstreams + " {\n")
		if p.Toggles.TLSSkipVerify {
			b.WriteString("\t\ttransport http {\n")
			b.WriteString("\t\t\ttls_insecure_skip_verify\n")
			b.WriteString("\t\t}\n")
		}
		if p.Toggles.WebSocketPassthru {
			b.WriteString("\t\tflush_interval -1\n")
		}
		if p.Toggles.LoadBalancing.Enabled {
			b.WriteString("\t\tlb_policy " + lbStrategy + "\n")
			if lbStrategy == "first" {
				b.WriteString("\t\tfail_duration 30s\n")
				b.WriteString("\t\tmax_fails 3\n")
			}
		}
		if p.Toggles.RequestHeaders.Enabled {
			if p.Toggles.RequestHeaders.HostOverride && p.Toggles.RequestHeaders.HostValue != "" {
				b.WriteString("\t\theader_up Host " + p.Toggles.RequestHeaders.HostValue + "\n")
			}
			if p.Toggles.RequestHeaders.Authorization && p.Toggles.RequestHeaders.AuthValue != "" {
				b.WriteString("\t\theader_up Authorization \"" + p.Toggles.RequestHeaders.AuthValue + "\"\n")
			}
		}
		b.WriteString("\t}\n")
	} else {
		b.WriteString("\treverse_proxy " + p.Upstream + "\n")
	}

	if p.Toggles.AccessLog != "" && logWriter != nil {
		b.WriteString("\tlog {\n")
		writeLogWriter(b, logWriter)
		b.WriteString("\t\tformat json\n")
		b.WriteString("\t}\n")
	}

	b.WriteString("}\n\n")
}
