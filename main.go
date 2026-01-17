package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"gopkg.in/yaml.v3"
)

// outWriter 用于输出一般执行信息；默认写入标准输出。
// outWriter is used to output general runtime information; defaults to standard output.
var (
	outWriter io.Writer = os.Stdout
	errWriter io.Writer = os.Stderr
)

// serviceConfig 定义服务启动所需的配置内容。
// serviceConfig defines the configuration required for service startup.
//
// 配置可同时通过 JSON 与 YAML 字段名称反序列化，
// Configuration can be deserialized using both JSON and YAML field names,
// 其中包含 NATS 连接配置与需要订阅的 NATS 主题。
// including NATS connection settings and the NATS subject to subscribe to.
type serviceConfig struct {
	// NatsConfig 保存 NATS 服务器连接相关配置。
	// NatsConfig holds the NATS server connection settings.
	NatsConfig nyanats.NatsConfig `json:"nats_config" yaml:"nats_config"`

	// NatsSubject 指定本服务需要订阅并处理请求的 NATS 主题。
	// NatsSubject specifies the NATS subject this service subscribes to for processing requests.
	NatsSubject string `json:"nats_subject" yaml:"nats_subject"`
}

// bridgeRequest 表示通过 NATS 传入的桥接请求数据。
// bridgeRequest represents the bridge request data received via NATS.
//
// 此结构通常对应到 HTTP 请求的主要信息，
// This structure typically corresponds to the main information of an HTTP request,
// 例如方法、路径、标头、Cookie、来源地址、查询参数与请求正文。
// such as method, path, headers, cookies, source address, query parameters, and request body.
type bridgeRequest struct {
	// Method 表示 HTTP 请求方法，例如 GET、POST。
	// Method represents the HTTP request method, e.g. GET, POST.
	Method string `json:"method"`

	// Path 表示请求路径。
	// Path represents the request path.
	Path string `json:"path"`

	// Headers 保存请求标头。
	// Headers holds the request headers.
	Headers map[string]string `json:"headers"`

	// Cookies 保存请求 Cookie。
	// Cookies holds the request cookies.
	Cookies map[string]string `json:"cookies"`

	// RemoteAddr 表示原始远端地址。
	// RemoteAddr represents the original remote address.
	RemoteAddr string `json:"remote_addr"`

	// IP 表示请求来源 IP。
	// IP represents the requesting client's IP address.
	IP string `json:"ip"`

	// Params 保存请求参数。
	// Params holds the request parameters.
	Params map[string]string `json:"params"`

	// Body 保存请求正文。
	// Body holds the request body.
	Body string `json:"body"`
}

// bridgeResponse 表示回传给桥接层的响应数据。
// bridgeResponse represents the response data returned to the bridge layer.
//
// 此结构通常对应到 HTTP 响应的状态码、标头与正文内容。
// This structure typically corresponds to the HTTP response status code, headers, and body content.
type bridgeResponse struct {
	// StatusCode 表示 HTTP 响应状态码。
	// StatusCode represents the HTTP response status code.
	StatusCode int `json:"status_code"`

	// Headers 保存响应标头。
	// Headers holds the response headers.
	Headers map[string]string `json:"headers"`

	// Body 保存响应正文。
	// Body holds the response body.
	Body string `json:"body"`
}

// loadConfig 加载并解析服务配置文件。
// loadConfig loads and parses the service configuration file.
//
// 若 configPath 为空，会依照当前可执行文件名称推导默认 YAML 配置文件名。
// If configPath is empty, it derives the default YAML config file name from the executable name.
// 例如可执行文件为 app，则默认读取 app.yaml。
// For example, if the executable is named app, it defaults to reading app.yaml.
func loadConfig(configPath string) (*serviceConfig, error) {
	if configPath == "" {
		// 未指定配置文件路径时，尝试取得当前可执行文件路径。
		// When no config path is specified, attempt to get the current executable path.
		exePath, err := os.Executable()
		if err != nil {
			// 若无法取得可执行文件路径，则退回使用启动参数中的程序名称。
			// If the executable path cannot be obtained, fall back to the program name from the startup arguments.
			exePath = os.Args[0]
		}

		// 依照可执行文件名称生成同名 YAML 配置文件路径。
		// Generate a YAML config file path with the same name as the executable.
		exeBase := filepath.Base(exePath)
		configPath = strings.TrimSuffix(exeBase, filepath.Ext(exeBase)) + ".yaml"
	}

	// 读取 YAML 配置文件内容。
	// Read the YAML config file content.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w (path: %s)", err, configPath)
	}

	// 将 YAML 内容解析为服务配置结构。
	// Parse the YAML content into the service config structure.
	var cfg serviceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// main 是服务入口点。
// main is the service entry point.
//
// 此函数负责解析命令行参数、初始化日志输出、加载配置、
// This function is responsible for parsing command-line arguments, initializing log output, loading configuration,
// 建立 NATS 连接、订阅请求主题，并在收到系统中断信号时优雅关闭服务。
// establishing a NATS connection, subscribing to the request subject, and gracefully shutting down on interrupt signal.
func main() {
	var configPath string
	var logFilePath string

	// 注册命令行参数：
	// Register command-line arguments:
	// -c 指定 YAML 配置文件路径。
	// -c specifies the YAML config file path.
	// -o 指定日志输出文件路径。
	// -o specifies the log output file path.
	flag.StringVar(&configPath, "c", "", "yaml config file")
	flag.StringVar(&logFilePath, "o", "", "log output file path")
	flag.Parse()

	if logFilePath != "" {
		// 若指定日志文件，则以附加模式打开或创建该文件。
		// If a log file is specified, open or create it in append mode.
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to open log file: %v\n", err)
			os.Exit(1)
		}
		defer logFile.Close()

		// 同时将一般输出与错误输出写入终端与日志文件。
		// Write both standard output and error output to the terminal and log file simultaneously.
		outWriter = io.MultiWriter(os.Stdout, logFile)
		errWriter = io.MultiWriter(os.Stderr, logFile)
	}

	// 加载服务配置。
	// Load the service configuration.
	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(errWriter, "[ERROR] %v\n", err)
		os.Exit(1)
	}

	// 输出当前 NATS 连接信息与订阅主题，方便启动时确认配置。
	// Output current NATS connection info and subscribed topic for easy config verification at startup.
	fmt.Fprintf(outWriter, "[INFO] NATS server: %s:%d\n", cfg.NatsConfig.NatsServerHost, cfg.NatsConfig.NatsServerPort)
	fmt.Fprintf(outWriter, "[INFO] Subscribed topic: %s\n", cfg.NatsSubject)

	// 建立 NATS 用 logger 与 NATS client。
	// Create NATS logger and NATS client.
	natsLogger := log.New(outWriter, "[NATS] ", 0)
	natsClient := nyanats.NewC(cfg.NatsConfig, natsLogger)

	// 检查 NATS 初始化或连接状态。
	// Check NATS initialization or connection status.
	if err := natsClient.Error(); err != nil {
		fmt.Fprintf(errWriter, "[ERROR] NATS connection failed: %v\n", err)
		os.Exit(1)
	}

	// 订阅指定主题，并在收到消息时解析桥接请求与回传桥接响应。
	// Subscribe to the specified subject, and on receiving a message, parse the bridge request and return a bridge response.
	err = natsClient.Subscribe(cfg.NatsSubject, func(m string) string {
		var req bridgeRequest

		// 将 NATS 消息内容解析为桥接请求。
		// Parse the NATS message content into a bridge request.
		if err := json.Unmarshal([]byte(m), &req); err != nil {
			fmt.Fprintf(errWriter, "[ERROR] Failed to parse request: %v\n", err)

			// 若请求格式无效，回传 400 错误响应。
			// If the request format is invalid, return a 400 error response.
			resp, _ := json.Marshal(bridgeResponse{
				StatusCode: 400,
				Headers:    map[string]string{"Content-Type": "application/json; charset=utf-8"},
				Body:       `{"error":"invalid request"}`,
			})
			return string(resp)
		}

		// 记录收到的请求方法、路径与来源 IP。
		// Log the received request method, path, and source IP.
		fmt.Fprintf(outWriter, "[INFO] Received request: %s %s from %s\n", req.Method, req.Path, req.IP)

		// 调用实际请求处理逻辑。
		// Call the actual request processing logic.
		result := handlePing(&req)

		// 将处理结果序列化后包装为桥接响应。
		// Serialize the processing result and wrap it as a bridge response.
		respBody, _ := json.Marshal(result)
		resp, _ := json.Marshal(bridgeResponse{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "application/json; charset=utf-8"},
			Body:       string(respBody),
		})
		return string(resp)
	})

	if err != nil {
		fmt.Fprintf(errWriter, "[ERROR] Subscribe failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(outWriter, "[INFO] Service started, waiting for requests... (Ctrl+C to exit)\n")

	// 建立系统信号通道，用于等待 Ctrl+C 或终止信号。
	// Create a system signal channel to wait for Ctrl+C or termination signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 收到结束信号后，开始优雅关闭流程。
	// After receiving the termination signal, begin the graceful shutdown process.
	fmt.Fprintf(outWriter, "[INFO] Shutting down...\n")
	if err := natsClient.UnsubscribeAll(); err != nil {
		fmt.Fprintf(errWriter, "[ERROR] Unsubscribe failed: %v\n", err)
	}

	// 关闭 NATS 连接并完成服务退出。
	// Close the NATS connection and complete service exit.
	natsClient.Close()
	fmt.Fprintf(outWriter, "[INFO] Shut down complete\n")
}
