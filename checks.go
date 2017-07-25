package main

import (
	//"fmt"
	"strconv"
)

func IsValidPort(p string) bool {
	i, err := strconv.Atoi(p)
	if err != nil || i < 0 || i > 65535 {
		return false
	}
	return true
}
