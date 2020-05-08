package utils

import (
	"net"
	"time"
)

func Telnet(host string, port string) bool {
	success := false
	timeout := time.Second
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		success = false
	} else {
		success = true
	}

	if conn != nil {
		defer conn.Close()
	}

	return success
}
