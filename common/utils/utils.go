package utils

import (
	"bytes"
	"github.com/pkg/errors"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"strings"
	"text/template"
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

func ParseTemplate(strtmpl string, obj ...interface{}) ([]byte, error) {
	if obj == nil || len(obj) == 0 {
		return []byte(strtmpl), nil
	}

	var buf bytes.Buffer
	tmpl, err := template.New("template").Parse(strtmpl)
	if err != nil {
		return nil, errors.Wrap(err, "error when parsing template")
	}
	err = tmpl.Execute(&buf, obj[0])
	if err != nil {
		return nil, errors.Wrap(err, "error when executing template")
	}
	return buf.Bytes(), nil
}

func WriteFile(fileName string, b []byte) error {
	var dir string
	arr := strings.Split(fileName, "/")

	if len(arr) > 1 {
		if !strings.Contains(runtime.GOOS, "windows") {
			dir += "/"
		}

		for i, d := range arr {
			if i < len(arr)-1 {
				dir += d + "/"
			}
		}

		if _, err := os.Stat(dir); err != nil {
			errMkdir := os.MkdirAll(dir, 0711)
			if errMkdir != nil {
				return errMkdir
			}
		}
	}

	err := ioutil.WriteFile(fileName, b, 0644)
	if err != nil {
		return err
	}

	return nil
}