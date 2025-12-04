package main

import (
	"fmt"
	"net"
	"time"
)

func CheckPing(target string) MonitorResult {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(target, "80"), 3*time.Second)
	if err != nil {
		return MonitorResult{Target: target, Status: false, Message: fmt.Sprintf("Error (%v)", err), Timestamp: time.Now()}
	}
	defer conn.Close()
	return MonitorResult{Target: target, Status: true, Message: fmt.Sprintf("Online (%v)", time.Since(start)), Timestamp: time.Now()}
}
