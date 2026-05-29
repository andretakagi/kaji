// Caddyfile generation and parsing. Converts between Caddy JSON and Caddyfile text.
package caddy

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
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
	HTTPPort       int
	HTTPSPort      int
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
				HTTPPort  *int                   `json:"http_port,omitempty"`
				HTTPSPort *int                   `json:"https_port,omitempty"`
				Servers   map[string]caddyServer `json:"servers"`
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

	if full.Apps.HTTP.HTTPPort != nil {
		cfg.HTTPPort = *full.Apps.HTTP.HTTPPort
	}
	if full.Apps.HTTP.HTTPSPort != nil {
		cfg.HTTPSPort = *full.Apps.HTTP.HTTPSPort
	}

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

	// Parse error pages per server before building domains, so we can attach
	// them to the right DomainParams.
	serverErrorPages := parseServerErrorPages(raw)

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
			if eps, ok := serverErrorPages[name][params.Domain]; ok {
				params.Toggles.ErrorPages = eps
			}
			cs.Domains = append(cs.Domains, params)
		}
		cfg.Servers[name] = cs
	}

	return cfg, nil
}

func GenerateCaddyfile(raw json.RawMessage, logFile string, logSkipRules map[string]LogSkipRule, forwardAuth *ForwardAuthConfig) (string, error) {
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
			var skipRule *LogSkipRule
			if route.Toggles.AccessLog != "" && logSkipRules != nil {
				if r, ok := logSkipRules[route.Toggles.AccessLog]; ok {
					skipRule = &r
				}
			}
			writeSiteBlock(&b, route, logWriter, skipRule, forwardAuth)
		}
	}

	return b.String(), nil
}

func writeGlobalOptions(b *strings.Builder, cfg *caddyfileConfig) {
	var lines []string

	if cfg.HTTPPort != 0 && cfg.HTTPPort != 80 {
		lines = append(lines, fmt.Sprintf("http_port %d", cfg.HTTPPort))
	}
	if cfg.HTTPSPort != 0 && cfg.HTTPSPort != 443 {
		lines = append(lines, fmt.Sprintf("https_port %d", cfg.HTTPSPort))
	}

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

func writeSiteBlock(b *strings.Builder, p DomainParams, logWriter *caddyfileLogWriter, skipRules *LogSkipRule, forwardAuth *ForwardAuthConfig) {
	if p.Toggles.ForceHTTPS {
		b.WriteString("http://" + p.Domain + " {\n")
		b.WriteString("\tredir https://{host}{uri} 301\n")
		b.WriteString("}\n\n")
	}

	b.WriteString(p.Domain + " {\n")

	if p.Toggles.Compression {
		b.WriteString("\tencode gzip zstd\n")
	}

	if p.Toggles.RequestBodyMaxSize != "" {
		b.WriteString("\trequest_body {\n")
		b.WriteString("\t\tmax_size " + p.Toggles.RequestBodyMaxSize + "\n")
		b.WriteString("\t}\n")
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

	if p.Toggles.Headers.Response.Enabled && len(p.Toggles.Headers.Response.Custom) > 0 {
		writeCustomResponseHeaders(b, p.Toggles.Headers.Response)
	}

	if p.Toggles.Headers.Request.Enabled {
		writeRequestHeaders(b, p.Toggles.Headers.Request)
	}

	if p.Toggles.Auth.Mode == "basic" && p.Toggles.Auth.BasicAuth.Username != "" {
		b.WriteString("\tbasic_auth {\n")
		b.WriteString("\t\t" + p.Toggles.Auth.BasicAuth.Username + " " + p.Toggles.Auth.BasicAuth.PasswordHash + "\n")
		b.WriteString("\t}\n")
	}

	if p.Toggles.Auth.Mode == "forward" && forwardAuth != nil && forwardAuth.Enabled {
		writeForwardAuthBlock(b, forwardAuth)
	}

	if p.Toggles.IPFiltering.Enabled && len(p.IPListIPs) > 0 {
		writeIPFilteringBlock(b, p)
	}

	matcher := ""
	if len(p.MethodMatch) > 0 {
		b.WriteString("\t@methods method " + strings.Join(p.MethodMatch, " ") + "\n")
		matcher = "@methods "
	}

	switch p.HandlerType {
	case "file_server":
		writeFileServerDirective(b, p.HandlerConfig, matcher)
	case "redirect":
		writeRedirectDirective(b, p.HandlerConfig, matcher)
	case "static_response":
		writeStaticResponseDirective(b, p.HandlerConfig, matcher)
	case "error":
		writeErrorDirective(b, p.HandlerConfig, matcher)
	default:
		writeReverseProxyDirective(b, p, matcher)
	}

	if len(p.Toggles.ErrorPages) > 0 {
		writeHandleErrorsBlock(b, p.Toggles.ErrorPages)
	}

	if skipRules != nil && hasSkipConditions(*skipRules) {
		writeSkipLogBlock(b, *skipRules)
	}

	if p.Toggles.AccessLog != "" && logWriter != nil {
		b.WriteString("\tlog {\n")
		writeLogWriter(b, logWriter)
		b.WriteString("\t\tformat json\n")
		b.WriteString("\t}\n")
	}

	b.WriteString("}\n\n")
}

func hasSkipConditions(rules LogSkipRule) bool {
	if rules.Mode == "advanced" {
		return len(rules.AdvancedRaw) > 0 && string(rules.AdvancedRaw) != "null"
	}
	return len(rules.Conditions) > 0
}

func writeSkipLogBlock(b *strings.Builder, rules LogSkipRule) {
	b.WriteString("\t@logskip {\n")

	if rules.Mode == "advanced" && len(rules.AdvancedRaw) > 0 {
		var sets []map[string]json.RawMessage
		if json.Unmarshal(rules.AdvancedRaw, &sets) == nil {
			for _, set := range sets {
				writeAdvancedMatcherLines(b, set)
			}
		}
	} else {
		var paths []string
		var regexes []string
		for _, cond := range rules.Conditions {
			switch cond.Type {
			case "path":
				paths = append(paths, cond.Value)
			case "path_regexp":
				regexes = append(regexes, cond.Value)
			case "header":
				b.WriteString("\t\theader " + cond.Key + " " + cond.Value + "\n")
			case "remote_ip":
				b.WriteString("\t\tremote_ip " + cond.Value + "\n")
			}
		}
		if len(paths) > 0 {
			b.WriteString("\t\tpath " + strings.Join(paths, " ") + "\n")
		}
		for _, re := range regexes {
			b.WriteString("\t\tpath_regexp " + re + "\n")
		}
	}

	b.WriteString("\t}\n")
	b.WriteString("\tskip_log @logskip\n")
}

func writeForwardAuthBlock(b *strings.Builder, cfg *ForwardAuthConfig) {
	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return
	}

	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	upstream := host + ":" + port

	b.WriteString("\tforward_auth " + upstream + " {\n")
	b.WriteString("\t\turi " + parsed.RequestURI() + "\n")

	if parsed.Scheme == "https" {
		b.WriteString("\t\ttransport http {\n")
		b.WriteString("\t\t\ttls\n")
		b.WriteString("\t\t}\n")
	}

	headers := ForwardAuthPresetHeaders(cfg.Provider)
	if len(headers) > 0 {
		b.WriteString("\t\tcopy_headers " + strings.Join(headers, " ") + "\n")
	}

	b.WriteString("\t}\n")
}

func writeAdvancedMatcherLines(b *strings.Builder, set map[string]json.RawMessage) {
	if pathRaw, ok := set["path"]; ok {
		var paths []string
		if json.Unmarshal(pathRaw, &paths) == nil && len(paths) > 0 {
			b.WriteString("\t\tpath " + strings.Join(paths, " ") + "\n")
		}
	}
	if regexpRaw, ok := set["path_regexp"]; ok {
		var re struct {
			Pattern string `json:"pattern"`
		}
		if json.Unmarshal(regexpRaw, &re) == nil && re.Pattern != "" {
			b.WriteString("\t\tpath_regexp " + re.Pattern + "\n")
		}
	}
	if headerRaw, ok := set["header"]; ok {
		var headers map[string][]string
		if json.Unmarshal(headerRaw, &headers) == nil {
			for key, vals := range headers {
				for _, v := range vals {
					b.WriteString("\t\theader " + key + " " + v + "\n")
				}
			}
		}
	}
	if ipRaw, ok := set["remote_ip"]; ok {
		var ip struct {
			Ranges []string `json:"ranges"`
		}
		if json.Unmarshal(ipRaw, &ip) == nil {
			for _, r := range ip.Ranges {
				b.WriteString("\t\tremote_ip " + r + "\n")
			}
		}
	}

	known := map[string]bool{"path": true, "path_regexp": true, "header": true, "remote_ip": true}
	keys := make([]string, 0, len(set))
	for k := range set {
		if !known[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("\t\t# unsupported matcher: " + k + "\n")
	}
}

func writeIPFilteringBlock(b *strings.Builder, p DomainParams) {
	matcher := p.Toggles.IPFiltering.Matcher
	if matcher == "" {
		matcher = "remote_ip"
	}
	ips := strings.Join(p.IPListIPs, " ")

	if p.IPListType == "whitelist" {
		b.WriteString("\t@blocked not " + matcher + " " + ips + "\n")
	} else {
		b.WriteString("\t@blocked " + matcher + " " + ips + "\n")
	}
	b.WriteString("\trespond @blocked 403\n")
}

func writeReverseProxyDirective(b *strings.Builder, p DomainParams, matcher string) {
	lbStrategy := p.Toggles.LoadBalancing.Strategy
	if p.Toggles.LoadBalancing.Enabled && lbStrategy == "" {
		lbStrategy = "round_robin"
	}

	var rpCfg ReverseProxyConfig
	if len(p.HandlerConfig) > 0 {
		json.Unmarshal(p.HandlerConfig, &rpCfg)
	}

	if rpCfg.StripPathPrefix != "" {
		b.WriteString("\turi strip_prefix " + rpCfg.StripPathPrefix + "\n")
	}
	if rpCfg.PrependPathPrefix != "" {
		b.WriteString("\trewrite * " + rpCfg.PrependPathPrefix + "{http.request.uri.path}\n")
	}

	hasHeaderUp := rpCfg.HeaderUp.Enabled &&
		((rpCfg.HeaderUp.HostOverride && rpCfg.HeaderUp.HostValue != "") ||
			(rpCfg.HeaderUp.Authorization && rpCfg.HeaderUp.AuthValue != ""))
	hasHeaderDown := rpCfg.HeaderDown.Enabled &&
		(rpCfg.HeaderDown.StripServer || rpCfg.HeaderDown.StripPoweredBy)
	hasHealthChecks := rpCfg.HealthChecks.Enabled &&
		(rpCfg.HealthChecks.Active.Enabled || rpCfg.HealthChecks.Passive.Enabled)
	needsBlock := p.Toggles.TLSSkipVerify || p.Toggles.WebSocketPassthru ||
		p.Toggles.LoadBalancing.Enabled || hasHeaderUp || hasHeaderDown || hasHealthChecks

	if needsBlock {
		allUpstreams := p.Upstream
		if p.Toggles.LoadBalancing.Enabled {
			for _, u := range p.Toggles.LoadBalancing.Upstreams {
				allUpstreams += " " + u
			}
		}
		b.WriteString("\treverse_proxy " + matcher + allUpstreams + " {\n")
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
			if lbStrategy == "first" && !rpCfg.HealthChecks.Passive.Enabled {
				b.WriteString("\t\tfail_duration 30s\n")
				b.WriteString("\t\tmax_fails 3\n")
			}
		}
		if hasHealthChecks {
			writeHealthChecks(b, rpCfg.HealthChecks)
		}
		if rpCfg.HeaderUp.Enabled {
			if rpCfg.HeaderUp.HostOverride && rpCfg.HeaderUp.HostValue != "" {
				b.WriteString("\t\theader_up Host " + rpCfg.HeaderUp.HostValue + "\n")
			}
			if rpCfg.HeaderUp.Authorization && rpCfg.HeaderUp.AuthValue != "" {
				b.WriteString("\t\theader_up Authorization \"" + rpCfg.HeaderUp.AuthValue + "\"\n")
			}
		}
		if rpCfg.HeaderDown.Enabled {
			if rpCfg.HeaderDown.StripServer {
				b.WriteString("\t\theader_down -Server\n")
			}
			if rpCfg.HeaderDown.StripPoweredBy {
				b.WriteString("\t\theader_down -X-Powered-By\n")
			}
		}
		b.WriteString("\t}\n")
	} else {
		b.WriteString("\treverse_proxy " + matcher + p.Upstream + "\n")
	}
}

func writeFileServerDirective(b *strings.Builder, handlerConfig json.RawMessage, matcher string) {
	var cfg FileServerConfig
	if len(handlerConfig) > 0 {
		json.Unmarshal(handlerConfig, &cfg)
	}

	if cfg.Root != "" {
		rootMatcher := "* "
		if matcher != "" {
			rootMatcher = matcher
		}
		b.WriteString("\troot " + rootMatcher + cfg.Root + "\n")
	}

	if cfg.Browse || len(cfg.Hide) > 0 || len(cfg.IndexNames) > 0 {
		b.WriteString("\tfile_server " + matcher + "{\n")
		if cfg.Browse {
			b.WriteString("\t\tbrowse\n")
		}
		if len(cfg.IndexNames) > 0 {
			b.WriteString("\t\tindex " + strings.Join(cfg.IndexNames, " ") + "\n")
		}
		for _, h := range cfg.Hide {
			b.WriteString("\t\thide " + h + "\n")
		}
		b.WriteString("\t}\n")
	} else if matcher != "" {
		b.WriteString("\tfile_server " + strings.TrimSpace(matcher) + "\n")
	} else {
		b.WriteString("\tfile_server\n")
	}
}

func writeRedirectDirective(b *strings.Builder, handlerConfig json.RawMessage, matcher string) {
	var cfg RedirectConfig
	if len(handlerConfig) > 0 {
		json.Unmarshal(handlerConfig, &cfg)
	}

	statusCode := cfg.StatusCode
	if statusCode == "" {
		statusCode = "302"
	}

	target := cfg.TargetURL
	if cfg.PreservePath {
		target = strings.TrimRight(target, "/") + "{uri}"
	}

	b.WriteString("\tredir " + matcher + target + " " + statusCode + "\n")
}

func writeStaticResponseDirective(b *strings.Builder, handlerConfig json.RawMessage, matcher string) {
	var cfg StaticResponseConfig
	if len(handlerConfig) > 0 {
		json.Unmarshal(handlerConfig, &cfg)
	}

	if cfg.Close {
		if matcher != "" {
			b.WriteString("\tabort " + strings.TrimSpace(matcher) + "\n")
		} else {
			b.WriteString("\tabort\n")
		}
		return
	}

	statusCode := cfg.StatusCode
	if statusCode == "" {
		statusCode = "200"
	}

	if cfg.Body != "" {
		b.WriteString("\trespond " + matcher + "\"" + cfg.Body + "\" " + statusCode + "\n")
	} else {
		b.WriteString("\trespond " + matcher + statusCode + "\n")
	}
}

func writeErrorDirective(b *strings.Builder, handlerConfig json.RawMessage, matcher string) {
	var cfg ErrorConfig
	if len(handlerConfig) > 0 {
		json.Unmarshal(handlerConfig, &cfg)
	}

	statusCode := cfg.StatusCode
	if statusCode == "" {
		statusCode = "500"
	}

	if cfg.Message != "" {
		b.WriteString("\terror " + matcher + "\"" + cfg.Message + "\" " + statusCode + "\n")
	} else {
		b.WriteString("\terror " + matcher + statusCode + "\n")
	}
}

func parseServerErrorPages(raw json.RawMessage) map[string]map[string][]ErrorPage {
	var top struct {
		Apps struct {
			HTTP struct {
				Servers map[string]struct {
					Errors *struct {
						Routes []struct {
							Match []struct {
								Host       []string `json:"host"`
								Expression string   `json:"expression"`
							} `json:"match"`
							Handle []struct {
								Handler string              `json:"handler"`
								Body    string              `json:"body"`
								Headers map[string][]string `json:"headers"`
							} `json:"handle"`
						} `json:"routes"`
					} `json:"errors"`
				} `json:"servers"`
			} `json:"http"`
		} `json:"apps"`
	}
	if json.Unmarshal(raw, &top) != nil {
		return nil
	}

	result := make(map[string]map[string][]ErrorPage)
	for name, srv := range top.Apps.HTTP.Servers {
		if srv.Errors == nil || len(srv.Errors.Routes) == 0 {
			continue
		}
		domainErrors := make(map[string][]ErrorPage)
		for _, route := range srv.Errors.Routes {
			if len(route.Match) == 0 || len(route.Match[0].Host) == 0 {
				continue
			}
			host := route.Match[0].Host[0]
			statusCode := parseStatusExpression(route.Match[0].Expression)

			var body, contentType string
			if len(route.Handle) > 0 {
				body = route.Handle[0].Body
				if ct, ok := route.Handle[0].Headers["Content-Type"]; ok && len(ct) > 0 {
					contentType = ct[0]
				}
			}

			domainErrors[host] = append(domainErrors[host], ErrorPage{
				StatusCode:  statusCode,
				Body:        body,
				ContentType: contentType,
			})
		}
		result[name] = domainErrors
	}
	return result
}

func writeHandleErrorsBlock(b *strings.Builder, errorPages []ErrorPage) {
	b.WriteString("\thandle_errors {\n")
	for i, ep := range errorPages {
		matcherName := fmt.Sprintf("@err%d", i)
		expr, err := buildStatusExpression(ep.StatusCode)
		if err != nil {
			continue
		}
		b.WriteString("\t\t" + matcherName + " expression `" + expr + "`\n")

		contentType := ep.ContentType
		if contentType == "" {
			contentType = "text/html"
		}
		b.WriteString("\t\theader " + matcherName + " Content-Type \"" + contentType + "\"\n")
		if ep.Body != "" {
			b.WriteString("\t\trespond " + matcherName + " \"" + ep.Body + "\" {http.error.status_code}\n")
		} else {
			b.WriteString("\t\trespond " + matcherName + " {http.error.status_code}\n")
		}
	}
	b.WriteString("\t}\n")
}

var builtinRequestHeaderValues = map[string]string{
	"X-Forwarded-For":   "{http.request.remote.host}",
	"X-Real-IP":         "{http.request.remote.host}",
	"X-Forwarded-Proto": "{http.request.scheme}",
	"X-Forwarded-Host":  "{http.request.host}",
	"X-Request-ID":      "{http.request.uuid}",
}

func writeRequestHeaders(b *strings.Builder, req DomainRequestHeaders) {
	entries := mergeEntries(req.Builtin, req.Custom)
	if len(entries) > 0 {
		writeHeaderEntries(b, entries, "request_header")
		return
	}

	// Fallback for data that only has boolean flags (not from a parsed route).
	if req.XForwardedFor {
		b.WriteString("\trequest_header X-Forwarded-For \"" + builtinRequestHeaderValues["X-Forwarded-For"] + "\"\n")
	}
	if req.XRealIP {
		b.WriteString("\trequest_header X-Real-IP \"" + builtinRequestHeaderValues["X-Real-IP"] + "\"\n")
	}
	if req.XForwardedProto {
		b.WriteString("\trequest_header X-Forwarded-Proto \"" + builtinRequestHeaderValues["X-Forwarded-Proto"] + "\"\n")
	}
	if req.XForwardedHost {
		b.WriteString("\trequest_header X-Forwarded-Host \"" + builtinRequestHeaderValues["X-Forwarded-Host"] + "\"\n")
	}
	if req.XRequestID {
		b.WriteString("\trequest_header X-Request-ID \"" + builtinRequestHeaderValues["X-Request-ID"] + "\"\n")
	}
}

func writeHeaderEntries(b *strings.Builder, entries []HeaderEntry, directive string) {
	for _, h := range entries {
		if !h.Enabled || h.Key == "" {
			continue
		}
		switch h.Operation {
		case "delete":
			b.WriteString("\t" + directive + " -" + h.Key + "\n")
		case "add":
			b.WriteString("\t" + directive + " +" + h.Key + " \"" + h.Value + "\"\n")
		case "replace":
			b.WriteString("\t" + directive + " " + h.Key + " \"" + h.Search + "\" \"" + h.Value + "\"\n")
		default:
			b.WriteString("\t" + directive + " " + h.Key + " \"" + h.Value + "\"\n")
		}
	}
}

func writeCustomResponseHeaders(b *strings.Builder, resp ResponseHeaders) {
	writeHeaderEntries(b, resp.Custom, "header")
}

func writeHealthChecks(b *strings.Builder, hc HealthCheckConfig) {
	if hc.Active.Enabled {
		if hc.Active.URI != "" {
			b.WriteString("\t\thealth_uri " + hc.Active.URI + "\n")
		}
		if hc.Active.Interval != "" {
			b.WriteString("\t\thealth_interval " + hc.Active.Interval + "\n")
		}
		if hc.Active.Timeout != "" {
			b.WriteString("\t\thealth_timeout " + hc.Active.Timeout + "\n")
		}
		if hc.Active.Port != 0 {
			b.WriteString("\t\thealth_port " + strconv.Itoa(hc.Active.Port) + "\n")
		}
		if hc.Active.ExpectStatus != 0 {
			b.WriteString("\t\thealth_status " + strconv.Itoa(hc.Active.ExpectStatus) + "\n")
		}
		if hc.Active.ExpectBody != "" {
			b.WriteString("\t\thealth_body \"" + hc.Active.ExpectBody + "\"\n")
		}
	}
	if hc.Passive.Enabled {
		if hc.Passive.FailDuration != "" {
			b.WriteString("\t\tfail_duration " + hc.Passive.FailDuration + "\n")
		}
		if hc.Passive.MaxFails != 0 {
			b.WriteString("\t\tmax_fails " + strconv.Itoa(hc.Passive.MaxFails) + "\n")
		}
		if len(hc.Passive.UnhealthyStatus) > 0 {
			codes := make([]string, len(hc.Passive.UnhealthyStatus))
			for i, c := range hc.Passive.UnhealthyStatus {
				codes[i] = strconv.Itoa(c)
			}
			b.WriteString("\t\tunhealthy_status " + strings.Join(codes, " ") + "\n")
		}
		if hc.Passive.UnhealthyLatency != "" {
			b.WriteString("\t\tunhealthy_latency " + hc.Passive.UnhealthyLatency + "\n")
		}
		if hc.Passive.UnhealthyRequestCount != 0 {
			b.WriteString("\t\tunhealthy_request_count " + strconv.Itoa(hc.Passive.UnhealthyRequestCount) + "\n")
		}
	}
}
