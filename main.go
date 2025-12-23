package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"gopkg.in/yaml.v3"
)

type serviceConfig struct {
	NatsConfig  nyanats.NatsConfig `json:"nats_config" yaml:"nats_config"`
	NatsSubject string             `json:"nats_subject" yaml:"nats_subject"`
}

type bridgeRequest struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers"`
	Cookies    map[string]string `json:"cookies"`
	RemoteAddr string            `json:"remote_addr"`
	IP         string            `json:"ip"`
	Params     map[string]string `json:"params"`
	Body       string            `json:"body"`
}

type bridgeResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

func loadConfig(configPath string) (*serviceConfig, error) {
	if configPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			exePath = os.Args[0]
		}
		exeBase := filepath.Base(exePath)
		configPath = strings.TrimSuffix(exeBase, filepath.Ext(exeBase)) + ".yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("讀取設定檔失敗: %w (path: %s)", err, configPath)
	}

	var cfg serviceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析設定檔失敗: %w", err)
	}

	return &cfg, nil
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "c", "", "yaml config file")
	flag.Parse()

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[INFO] NATS 伺服器: %s:%d\n", cfg.NatsConfig.NatsServerHost, cfg.NatsConfig.NatsServerPort)
	fmt.Printf("[INFO] 訂閱主題: %s\n", cfg.NatsSubject)

	natsLogger := log.New(os.Stdout, "[NATS] ", 0)
	natsClient := nyanats.NewC(cfg.NatsConfig, natsLogger)
	if err := natsClient.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] NATS 連線失敗: %v\n", err)
		os.Exit(1)
	}

	err = natsClient.Subscribe(cfg.NatsSubject, func(m string) string {
		var req bridgeRequest
		if err := json.Unmarshal([]byte(m), &req); err != nil {
			fmt.Printf("[ERROR] 解析請求失敗: %v\n", err)
			resp, _ := json.Marshal(bridgeResponse{
				StatusCode: 400,
				Headers:    map[string]string{"Content-Type": "application/json; charset=utf-8"},
				Body:       `{"error":"invalid request"}`,
			})
			return string(resp)
		}

		fmt.Printf("[INFO] 收到請求: %s %s from %s\n", req.Method, req.Path, req.IP)

		result := handlePing(&req)

		respBody, _ := json.Marshal(result)
		resp, _ := json.Marshal(bridgeResponse{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "application/json; charset=utf-8"},
			Body:       string(respBody),
		})
		return string(resp)
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 訂閱失敗: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[INFO] 服務已啟動，等待請求... (Ctrl+C 退出)")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("[INFO] 正在關閉...")
	if err := natsClient.UnsubscribeAll(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] 取消訂閱失敗: %v\n", err)
	}
	natsClient.Close()
	fmt.Println("[INFO] 已關閉")
}
