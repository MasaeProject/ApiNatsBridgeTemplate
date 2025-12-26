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

// outWriter 用於輸出一般執行資訊；預設寫入標準輸出。
var (
	outWriter io.Writer = os.Stdout
	errWriter io.Writer = os.Stderr
)

// serviceConfig 定義服務啟動所需的設定內容。
//
// 設定可同時透過 JSON 與 YAML 欄位名稱反序列化，
// 其中包含 NATS 連線設定與需要訂閱的 NATS 主題。
type serviceConfig struct {
	// NatsConfig 保存 NATS 伺服器連線相關設定。
	NatsConfig nyanats.NatsConfig `json:"nats_config" yaml:"nats_config"`

	// NatsSubject 指定本服務需要訂閱並處理請求的 NATS 主題。
	NatsSubject string `json:"nats_subject" yaml:"nats_subject"`
}

// bridgeRequest 表示透過 NATS 傳入的橋接請求資料。
//
// 此結構通常對應到 HTTP 請求的主要資訊，
// 例如方法、路徑、標頭、Cookie、來源位址、查詢參數與請求本文。
type bridgeRequest struct {
	// Method 表示 HTTP 請求方法，例如 GET、POST。
	Method string `json:"method"`

	// Path 表示請求路徑。
	Path string `json:"path"`

	// Headers 保存請求標頭。
	Headers map[string]string `json:"headers"`

	// Cookies 保存請求 Cookie。
	Cookies map[string]string `json:"cookies"`

	// RemoteAddr 表示原始遠端位址。
	RemoteAddr string `json:"remote_addr"`

	// IP 表示請求來源 IP。
	IP string `json:"ip"`

	// Params 保存請求參數。
	Params map[string]string `json:"params"`

	// Body 保存請求本文。
	Body string `json:"body"`
}

// bridgeResponse 表示回傳給橋接層的回應資料。
//
// 此結構通常對應到 HTTP 回應的狀態碼、標頭與本文內容。
type bridgeResponse struct {
	// StatusCode 表示 HTTP 回應狀態碼。
	StatusCode int `json:"status_code"`

	// Headers 保存回應標頭。
	Headers map[string]string `json:"headers"`

	// Body 保存回應本文。
	Body string `json:"body"`
}

// loadConfig 載入並解析服務設定檔。
//
// 若 configPath 為空，會依照目前可執行檔名稱推導預設 YAML 設定檔名稱。
// 例如可執行檔為 app，則預設讀取 app.yaml。
func loadConfig(configPath string) (*serviceConfig, error) {
	if configPath == "" {
		// 未指定設定檔路徑時，嘗試取得目前可執行檔路徑。
		exePath, err := os.Executable()
		if err != nil {
			// 若無法取得可執行檔路徑，則退回使用啟動參數中的程式名稱。
			exePath = os.Args[0]
		}

		// 依照可執行檔名稱產生同名 YAML 設定檔路徑。
		exeBase := filepath.Base(exePath)
		configPath = strings.TrimSuffix(exeBase, filepath.Ext(exeBase)) + ".yaml"
	}

	// 讀取 YAML 設定檔內容。
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("讀取設定檔失敗: %w (path: %s)", err, configPath)
	}

	// 將 YAML 內容解析為服務設定結構。
	var cfg serviceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析設定檔失敗: %w", err)
	}

	return &cfg, nil
}

// main 是服務入口點。
//
// 此函式負責解析命令列參數、初始化日誌輸出、載入設定、
// 建立 NATS 連線、訂閱請求主題，並在收到系統中斷訊號時優雅關閉服務。
func main() {
	var configPath string
	var logFilePath string

	// 註冊命令列參數：
	// -c 指定 YAML 設定檔路徑。
	// -o 指定日誌輸出檔案路徑。
	flag.StringVar(&configPath, "c", "", "yaml config file")
	flag.StringVar(&logFilePath, "o", "", "log output file path")
	flag.Parse()

	if logFilePath != "" {
		// 若指定日誌檔案，則以附加模式開啟或建立該檔案。
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] 無法開啟日誌檔案: %v\n", err)
			os.Exit(1)
		}
		defer logFile.Close()

		// 同時將一般輸出與錯誤輸出寫入終端機與日誌檔案。
		outWriter = io.MultiWriter(os.Stdout, logFile)
		errWriter = io.MultiWriter(os.Stderr, logFile)
	}

	// 載入服務設定。
	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(errWriter, "[ERROR] %v\n", err)
		os.Exit(1)
	}

	// 輸出目前 NATS 連線資訊與訂閱主題，方便啟動時確認設定。
	fmt.Fprintf(outWriter, "[INFO] NATS 伺服器: %s:%d\n", cfg.NatsConfig.NatsServerHost, cfg.NatsConfig.NatsServerPort)
	fmt.Fprintf(outWriter, "[INFO] 訂閱主題: %s\n", cfg.NatsSubject)

	// 建立 NATS 用 logger 與 NATS client。
	natsLogger := log.New(outWriter, "[NATS] ", 0)
	natsClient := nyanats.NewC(cfg.NatsConfig, natsLogger)

	// 檢查 NATS 初始化或連線狀態。
	if err := natsClient.Error(); err != nil {
		fmt.Fprintf(errWriter, "[ERROR] NATS 連線失敗: %v\n", err)
		os.Exit(1)
	}

	// 訂閱指定主題，並在收到訊息時解析橋接請求與回傳橋接回應。
	err = natsClient.Subscribe(cfg.NatsSubject, func(m string) string {
		var req bridgeRequest

		// 將 NATS 訊息內容解析為橋接請求。
		if err := json.Unmarshal([]byte(m), &req); err != nil {
			fmt.Fprintf(errWriter, "[ERROR] 解析請求失敗: %v\n", err)

			// 若請求格式無效，回傳 400 錯誤回應。
			resp, _ := json.Marshal(bridgeResponse{
				StatusCode: 400,
				Headers:    map[string]string{"Content-Type": "application/json; charset=utf-8"},
				Body:       `{"error":"invalid request"}`,
			})
			return string(resp)
		}

		// 記錄收到的請求方法、路徑與來源 IP。
		fmt.Fprintf(outWriter, "[INFO] 收到請求: %s %s from %s\n", req.Method, req.Path, req.IP)

		// 呼叫實際請求處理邏輯。
		result := handlePing(&req)

		// 將處理結果序列化後包裝為橋接回應。
		respBody, _ := json.Marshal(result)
		resp, _ := json.Marshal(bridgeResponse{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "application/json; charset=utf-8"},
			Body:       string(respBody),
		})
		return string(resp)
	})

	if err != nil {
		fmt.Fprintf(errWriter, "[ERROR] 訂閱失敗: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(outWriter, "[INFO] 服務已啟動，等待請求... (Ctrl+C 退出)\n")

	// 建立系統訊號通道，用於等待 Ctrl+C 或終止訊號。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 收到結束訊號後，開始優雅關閉流程。
	fmt.Fprintf(outWriter, "[INFO] 正在關閉...\n")
	if err := natsClient.UnsubscribeAll(); err != nil {
		fmt.Fprintf(errWriter, "[ERROR] 取消訂閱失敗: %v\n", err)
	}

	// 關閉 NATS 連線並完成服務退出。
	natsClient.Close()
	fmt.Fprintf(outWriter, "[INFO] 已關閉\n")
}
