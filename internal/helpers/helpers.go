package helpers

import (
	"fmt"
	"runtime"
	"strconv"
	"time"
)

func ValidPort(port string) bool {
	i, err := strconv.Atoi(port)
	return err == nil && i >= 1 && i <= 65535
}

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

func Ms2Float64(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func DurStr(d time.Duration) string {
	return fmt.Sprintf("%.2fms", Ms2Float64(d))
}
