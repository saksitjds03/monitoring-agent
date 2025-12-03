package main
import ("fmt"; "net/http"; "time")
func CheckHTTP(url string) MonitorResult {
start := time.Now(); client := http.Client{Timeout: 3 * time.Second}; resp, err := client.Get(url)
if err != nil { return MonitorResult{Target: url, Status: false, Message: "Connection Refused", Timestamp: time.Now()} }
defer resp.Body.Close()
if resp.StatusCode == 200 { return MonitorResult{Target: url, Status: true, Message: fmt.Sprintf("OK 200 (%v)", time.Since(start)), Timestamp: time.Now()} }
return MonitorResult{Target: url, Status: false, Message: fmt.Sprintf("Status: %d", resp.StatusCode), Timestamp: time.Now()}
}
