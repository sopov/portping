package helpers

import (
	"fmt"
	"runtime"
	"strconv"
	"time"
)

// NormalizePort parses a decimal TCP/UDP port and returns its canonical
// form ("0080" becomes "80"). Signs, spaces, zero and values above 65535 are
// rejected.
func NormalizePort(port string) (string, error) {
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return "", fmt.Errorf("invalid port `%s`", port)
	}
	return strconv.FormatUint(n, 10), nil
}

// ValidPort reports whether NormalizePort accepts port.
func ValidPort(port string) bool {
	_, err := NormalizePort(port)
	return err == nil
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
