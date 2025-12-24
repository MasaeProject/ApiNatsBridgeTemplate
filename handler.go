package main

import (
	"strconv"
	"time"
)

type pingResponse struct {
	Pong int64  `json:"pong"`
	IP   string `json:"ip"`
}

func handlePing(req *bridgeRequest) *pingResponse {
	var clientTimestampMs int64

	if ts, ok := req.Params["timestamp"]; ok && ts != "" {
		parsed, err := strconv.ParseInt(ts, 10, 64)
		if err == nil {
			clientTimestampMs = parsed
		}
	}

	nowMs := time.Now().UnixMilli()

	var pong int64
	if clientTimestampMs > 0 {
		pong = nowMs - clientTimestampMs
	} else {
		pong = nowMs
	}

	return &pingResponse{
		Pong: pong,
		IP:   req.IP,
	}
}
