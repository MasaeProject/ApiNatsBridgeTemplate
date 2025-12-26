package main

import (
	"strconv"
	"time"
)

// pingResponse 表示 ping 請求的回應資料。
//
// Pong 會依照請求參數中的 timestamp 決定回傳內容：
// 若有提供有效的客戶端時間戳，則回傳目前時間與該時間戳的差值；
// 若未提供有效時間戳，則回傳目前伺服器時間戳。
type pingResponse struct {
	// Pong 表示延遲毫秒數，或目前伺服器時間戳毫秒值。
	Pong int64 `json:"pong"`

	// IP 表示請求來源 IP。
	IP string `json:"ip"`
}

// handlePing 處理 ping 請求並產生回應。
//
// 此函式會從請求參數中讀取 timestamp，
// 若 timestamp 可成功解析為毫秒時間戳，則計算伺服器目前時間與客戶端時間戳的差值；
// 否則直接回傳伺服器目前的毫秒時間戳。
func handlePing(req *bridgeRequest) *pingResponse {
	// clientTimestampMs 保存客戶端傳入的毫秒時間戳。
	var clientTimestampMs int64

	// 嘗試從請求參數中取得 timestamp，並解析為 64 位元整數。
	if ts, ok := req.Params["timestamp"]; ok && ts != "" {
		parsed, err := strconv.ParseInt(ts, 10, 64)
		if err == nil {
			clientTimestampMs = parsed
		}
	}

	// 取得目前伺服器時間的毫秒時間戳。
	nowMs := time.Now().UnixMilli()

	// pong 保存最終要回傳的時間值或延遲值。
	var pong int64
	if clientTimestampMs > 0 {
		// 若客戶端提供有效時間戳，計算往返或處理時間差。
		pong = nowMs - clientTimestampMs
	} else {
		// 若沒有有效時間戳，直接回傳目前伺服器時間。
		pong = nowMs
	}

	// 回傳 ping 結果與請求來源 IP。
	return &pingResponse{
		Pong: pong,
		IP:   req.IP,
	}
}
