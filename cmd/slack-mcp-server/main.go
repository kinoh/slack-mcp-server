package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/server"
	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var defaultSseHost = "127.0.0.1"
var defaultSsePort = 13080

func main() {
	transports := newTransportFlag()
	var enabledToolsFlag string
	flag.Var(transports, "t", "Transport type (stdio, sse, http or sse,http)")
	flag.Var(transports, "transport", "Transport type (stdio, sse, http or sse,http)")
	flag.StringVar(&enabledToolsFlag, "e", "", "Comma-separated list of enabled tools (empty = all tools)")
	flag.StringVar(&enabledToolsFlag, "enabled-tools", "", "Comma-separated list of enabled tools (empty = all tools)")
	flag.Parse()

	transport, err := transports.canonical()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid transport type: %v\n", err)
		os.Exit(1)
	}

	if enabledToolsFlag == "" {
		enabledToolsFlag = os.Getenv("SLACK_MCP_ENABLED_TOOLS")
	}

	var enabledTools []string
	if enabledToolsFlag != "" {
		for _, tool := range strings.Split(enabledToolsFlag, ",") {
			tool = strings.TrimSpace(tool)
			if tool != "" {
				enabledTools = append(enabledTools, tool)
			}
		}
	}

	logger, err := newLogger(transport)
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	addMessageToolEnv := os.Getenv("SLACK_MCP_ADD_MESSAGE_TOOL")
	err = validateToolConfig(addMessageToolEnv)
	if err != nil {
		logger.Fatal("error in SLACK_MCP_ADD_MESSAGE_TOOL",
			zap.String("context", "console"),
			zap.Error(err),
		)
	}

	err = server.ValidateEnabledTools(enabledTools)
	if err != nil {
		logger.Fatal("error in SLACK_MCP_ENABLED_TOOLS",
			zap.String("context", "console"),
			zap.Error(err),
		)
	}

	p := provider.New(providerTransport(transport), logger)
	s := server.NewMCPServer(p, logger, enabledTools)

	go func() {
		var once sync.Once

		newUsersWatcher(p, &once, logger)()
		newChannelsWatcher(p, &once, logger)()
	}()

	switch transport {
	case "stdio":
		for {
			if ready, _ := p.IsReady(); ready {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err := s.ServeStdio(); err != nil {
			logger.Fatal("Server error",
				zap.String("context", "console"),
				zap.Error(err),
			)
		}
	case "sse":
		host := os.Getenv("SLACK_MCP_HOST")
		if host == "" {
			host = defaultSseHost
		}
		port := os.Getenv("SLACK_MCP_PORT")
		if port == "" {
			port = strconv.Itoa(defaultSsePort)
		}

		sseServer := s.ServeSSE(":" + port)
		logger.Info(
			fmt.Sprintf("SSE server listening on %s", fmt.Sprintf("%s:%s/sse", host, port)),
			zap.String("context", "console"),
			zap.String("host", host),
			zap.String("port", port),
		)

		if ready, _ := p.IsReady(); !ready {
			logger.Info("Slack MCP Server is still warming up caches",
				zap.String("context", "console"),
			)
		}

		if err := sseServer.Start(host + ":" + port); err != nil {
			logger.Fatal("Server error",
				zap.String("context", "console"),
				zap.Error(err),
			)
		}
	case "http":
		host := os.Getenv("SLACK_MCP_HOST")
		if host == "" {
			host = defaultSseHost
		}
		port := os.Getenv("SLACK_MCP_PORT")
		if port == "" {
			port = strconv.Itoa(defaultSsePort)
		}

		httpServer := s.ServeHTTP(":" + port)
		logger.Info(
			fmt.Sprintf("HTTP server listening on %s", fmt.Sprintf("%s:%s", host, port)),
			zap.String("context", "console"),
			zap.String("host", host),
			zap.String("port", port),
		)

		if ready, _ := p.IsReady(); !ready {
			logger.Info("Slack MCP Server is still warming up caches",
				zap.String("context", "console"),
			)
		}

		if err := httpServer.Start(host + ":" + port); err != nil {
			logger.Fatal("Server error",
				zap.String("context", "console"),
				zap.Error(err),
			)
		}
	case "sse,http", "http,sse":
		host := os.Getenv("SLACK_MCP_HOST")
		if host == "" {
			host = defaultSseHost
		}
		port := os.Getenv("SLACK_MCP_PORT")
		if port == "" {
			port = strconv.Itoa(defaultSsePort)
		}

		addr := host + ":" + port
		sseServer := s.ServeSSE(addr)
		httpServer := s.ServeHTTP(addr)

		mux := http.NewServeMux()
		mux.Handle("/sse", sseServer.SSEHandler())
		mux.Handle("/message", sseServer.MessageHandler())
		mux.Handle("/mcp", httpServer)

		handler := accessLogMiddleware(logger, mux)
		logger.Info(
			fmt.Sprintf("SSE and HTTP servers listening on %s", addr),
			zap.String("context", "console"),
			zap.String("host", host),
			zap.String("port", port),
			zap.String("sse_endpoint", fmt.Sprintf("%s/sse", addr)),
			zap.String("sse_message_endpoint", fmt.Sprintf("%s/message", addr)),
			zap.String("http_endpoint", fmt.Sprintf("%s/mcp", addr)),
		)

		if ready, _ := p.IsReady(); !ready {
			logger.Info("Slack MCP Server is still warming up caches",
				zap.String("context", "console"),
			)
		}

		httpSrv := &http.Server{
			Addr:    addr,
			Handler: handler,
		}
		if err := httpSrv.ListenAndServe(); err != nil {
			logger.Fatal("Server error",
				zap.String("context", "console"),
				zap.Error(err),
			)
		}
	default:
		logger.Fatal("Invalid transport type",
			zap.String("context", "console"),
			zap.String("transport", transport),
			zap.String("allowed", "stdio, sse, http, sse,http"),
		)
	}
}

type transportFlag struct {
	values []string
	set    bool
}

func newTransportFlag() *transportFlag {
	return &transportFlag{values: []string{"stdio"}}
}

func (f *transportFlag) String() string {
	return strings.Join(f.values, ",")
}

func (f *transportFlag) Set(value string) error {
	if !f.set {
		f.values = nil
		f.set = true
	}

	for _, transport := range strings.Split(value, ",") {
		transport = strings.TrimSpace(transport)
		if transport == "" {
			return fmt.Errorf("empty transport")
		}
		f.values = append(f.values, transport)
	}

	return nil
}

func (f *transportFlag) canonical() (string, error) {
	if len(f.values) == 0 {
		return "", fmt.Errorf("transport is required")
	}

	seen := map[string]bool{}
	for _, transport := range f.values {
		switch transport {
		case "stdio", "sse", "http":
			if seen[transport] {
				return "", fmt.Errorf("duplicate transport %q", transport)
			}
			seen[transport] = true
		default:
			return "", fmt.Errorf("unsupported transport %q; allowed values are stdio, sse, http and sse,http", transport)
		}
	}

	if seen["stdio"] && len(seen) > 1 {
		return "", fmt.Errorf("stdio cannot be combined with sse or http")
	}
	if seen["stdio"] {
		return "stdio", nil
	}
	if seen["sse"] && seen["http"] {
		return "sse,http", nil
	}
	if seen["sse"] {
		return "sse", nil
	}
	if seen["http"] {
		return "http", nil
	}

	return "", fmt.Errorf("transport is required")
}

func providerTransport(transport string) string {
	if transport == "sse,http" || transport == "http,sse" {
		return "http"
	}

	return transport
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func accessLogMiddleware(logger *zap.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		start := time.Now()

		next.ServeHTTP(recorder, r)

		logger.Info("HTTP access",
			zap.String("context", "http"),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", recorder.status),
			zap.Duration("duration", time.Since(start)),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
		)
	})
}

func newUsersWatcher(p *provider.ApiProvider, once *sync.Once, logger *zap.Logger) func() {
	return func() {
		logger.Info("Caching users collection...",
			zap.String("context", "console"),
		)

		if os.Getenv("SLACK_MCP_XOXP_TOKEN") == "demo" || (os.Getenv("SLACK_MCP_XOXC_TOKEN") == "demo" && os.Getenv("SLACK_MCP_XOXD_TOKEN") == "demo") {
			logger.Info("Demo credentials are set, skip",
				zap.String("context", "console"),
			)
			return
		}

		err := p.RefreshUsers(context.Background())
		if err != nil {
			logger.Fatal("Error booting provider",
				zap.String("context", "console"),
				zap.Error(err),
			)
		}

		ready, _ := p.IsReady()
		if ready {
			once.Do(func() {
				logger.Info("Slack MCP Server is fully ready",
					zap.String("context", "console"),
				)
			})
		}
	}
}

func newChannelsWatcher(p *provider.ApiProvider, once *sync.Once, logger *zap.Logger) func() {
	return func() {
		logger.Info("Caching channels collection...",
			zap.String("context", "console"),
		)

		if os.Getenv("SLACK_MCP_XOXP_TOKEN") == "demo" || (os.Getenv("SLACK_MCP_XOXC_TOKEN") == "demo" && os.Getenv("SLACK_MCP_XOXD_TOKEN") == "demo") {
			logger.Info("Demo credentials are set, skip.",
				zap.String("context", "console"),
			)
			return
		}

		err := p.RefreshChannels(context.Background())
		if err != nil {
			logger.Fatal("Error booting provider",
				zap.String("context", "console"),
				zap.Error(err),
			)
		}

		ready, _ := p.IsReady()
		if ready {
			once.Do(func() {
				logger.Info("Slack MCP Server is fully ready.",
					zap.String("context", "console"),
				)
			})
		}
	}
}

func validateToolConfig(config string) error {
	if config == "" || config == "true" || config == "1" {
		return nil
	}

	items := strings.Split(config, ",")
	hasNegated := false
	hasPositive := false

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "!") {
			hasNegated = true
		} else {
			hasPositive = true
		}
	}

	if hasNegated && hasPositive {
		return fmt.Errorf("cannot mix allowed and disallowed (! prefixed) channels")
	}

	return nil
}

func newLogger(transport string) (*zap.Logger, error) {
	atomicLevel := zap.NewAtomicLevelAt(zap.InfoLevel)
	if envLevel := os.Getenv("SLACK_MCP_LOG_LEVEL"); envLevel != "" {
		if err := atomicLevel.UnmarshalText([]byte(envLevel)); err != nil {
			fmt.Printf("Invalid log level '%s': %v, using 'info'\n", envLevel, err)
		}
	}

	useJSON := shouldUseJSONFormat()
	useColors := shouldUseColors() && !useJSON

	outputPath := "stdout"
	if transport == "stdio" {
		outputPath = "stderr"
	}

	var config zap.Config

	if useJSON {
		config = zap.Config{
			Level:            atomicLevel,
			Development:      false,
			Encoding:         "json",
			OutputPaths:      []string{outputPath},
			ErrorOutputPaths: []string{"stderr"},
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:       "timestamp",
				LevelKey:      "level",
				NameKey:       "logger",
				MessageKey:    "message",
				StacktraceKey: "stacktrace",
				EncodeLevel:   zapcore.LowercaseLevelEncoder,
				EncodeTime:    zapcore.RFC3339TimeEncoder,
				EncodeCaller:  zapcore.ShortCallerEncoder,
			},
		}
	} else {
		config = zap.Config{
			Level:            atomicLevel,
			Development:      true,
			Encoding:         "console",
			OutputPaths:      []string{outputPath},
			ErrorOutputPaths: []string{"stderr"},
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:          "timestamp",
				LevelKey:         "level",
				NameKey:          "logger",
				MessageKey:       "msg",
				StacktraceKey:    "stacktrace",
				EncodeLevel:      getConsoleLevelEncoder(useColors),
				EncodeTime:       zapcore.ISO8601TimeEncoder,
				EncodeCaller:     zapcore.ShortCallerEncoder,
				ConsoleSeparator: " | ",
			},
		}
	}

	logger, err := config.Build(zap.AddCaller())
	if err != nil {
		return nil, err
	}

	logger = logger.With(zap.String("app", "slack-mcp-server"))

	return logger, err
}

// shouldUseJSONFormat determines if JSON format should be used
func shouldUseJSONFormat() bool {
	if format := os.Getenv("SLACK_MCP_LOG_FORMAT"); format != "" {
		return strings.ToLower(format) == "json"
	}

	if env := os.Getenv("ENVIRONMENT"); env != "" {
		switch strings.ToLower(env) {
		case "production", "prod", "staging":
			return true
		case "development", "dev", "local":
			return false
		}
	}

	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" ||
		os.Getenv("DOCKER_CONTAINER") != "" ||
		os.Getenv("container") != "" {
		return true
	}

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return true
	}

	return false
}

func shouldUseColors() bool {
	if colorEnv := os.Getenv("SLACK_MCP_LOG_COLOR"); colorEnv != "" {
		return colorEnv == "true" || colorEnv == "1"
	}

	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}

	if env := os.Getenv("ENVIRONMENT"); env == "development" || env == "dev" {
		return isatty.IsTerminal(os.Stdout.Fd())
	}

	return isatty.IsTerminal(os.Stdout.Fd())
}

func getConsoleLevelEncoder(useColors bool) zapcore.LevelEncoder {
	if useColors {
		return zapcore.CapitalColorLevelEncoder
	}
	return zapcore.CapitalLevelEncoder
}
