package main

import (
	"strconv"
	"time"
)

// pingResponse 表示 ping 请求的响应数据。
// pingResponse represents the response data of a ping request.
//
// Pong 会依照请求参数中的 timestamp 决定回传内容：
// Pong will return different values based on the timestamp in the request parameters:
// 若有提供有效的客户端时间戳，则回传当前时间与该时间戳的差值；
// If a valid client timestamp is provided, it returns the difference between current time and that timestamp;
// 若未提供有效时间戳，则回传当前服务器时间戳。
// If no valid timestamp is provided, it returns the current server timestamp.
type pingResponse struct {
	// Pong 表示延迟毫秒数，或当前服务器时间戳毫秒值。
	// Pong represents the latency in milliseconds, or the current server timestamp in milliseconds.
	Pong int64 `json:"pong"`

	// IP 表示请求来源 IP。
	// IP represents the requesting client's IP address.
	IP string `json:"ip"`

	// ServerTime 表示当前服务器的系统毫秒级时间戳。
	// ServerTime represents the current server's system millisecond timestamp.
	ServerTime int64 `json:"servertime"`
}

// handlePing 处理 ping 请求并产生响应。
// handlePing processes a ping request and generates a response.
//
// 此函数会从请求参数中读取 timestamp，
// This function reads the timestamp from the request parameters,
// 若 timestamp 可成功解析为毫秒时间戳，则计算服务器当前时间与客户端时间戳的差值；
// If timestamp can be successfully parsed as a millisecond timestamp, it calculates the diff between server time and client timestamp;
// 否则直接回传服务器当前的毫秒时间戳。
// Otherwise it directly returns the server's current millisecond timestamp.
func handlePing(req *bridgeRequest) *pingResponse {
	// clientTimestampMs 保存客户端传入的毫秒时间戳。
	// clientTimestampMs holds the millisecond timestamp from the client.
	var clientTimestampMs int64

	// 尝试从请求参数中取得 timestamp，并解析为 64 位整数。
	// Attempt to get the timestamp from request parameters and parse it as a 64-bit integer.
	if ts, ok := req.Params["timestamp"]; ok && ts != "" {
		parsed, err := strconv.ParseInt(ts, 10, 64)
		if err == nil {
			clientTimestampMs = parsed
		}
	}

	// 取得当前服务器时间的毫秒时间戳。
	// Get the current server time in milliseconds.
	nowMs := time.Now().UnixMilli()

	// pong 保存最终要回传的时间值或延迟值。
	// pong holds the final time value or latency value to return.
	var pong int64
	if clientTimestampMs > 0 {
		// 若客户端提供有效时间戳，计算往返或处理时间差。
		// If the client provided a valid timestamp, calculate the round-trip or processing time difference.
		pong = nowMs - clientTimestampMs
	} else {
		// 若没有有效时间戳，直接回传当前服务器时间。
		// If no valid timestamp, directly return the current server time.
		pong = nowMs
	}

	// 回传 ping 结果、请求来源 IP 与服务器当前时间戳。
	// Return the ping result, requesting client IP, and server current timestamp.
	return &pingResponse{
		Pong:       pong,
		IP:         req.IP,
		ServerTime: nowMs,
	}
}
