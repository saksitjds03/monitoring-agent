package main
import ("fmt"; "os/exec"; "strings"; "time")
func CheckProcess(keyword string) MonitorResult {
start := time.Now(); cmd := exec.Command("wmic", "process", "where", "name='node.exe'", "get", "commandline"); output, err := cmd.Output()
if err != nil { return MonitorResult{Target: keyword, Status: false, Message: "Not Found", Timestamp: time.Now()} }
if strings.Contains(string(output), keyword) { return MonitorResult{Target: keyword, Status: true, Message: fmt.Sprintf("Running (%v)", time.Since(start)), Timestamp: time.Now()} }
return MonitorResult{Target: keyword, Status: false, Message: "Not Found", Timestamp: time.Now()}
}
