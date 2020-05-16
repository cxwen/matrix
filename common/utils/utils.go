package utils

import (
	"bytes"
	"fmt"
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
	con, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", host, port), time.Duration(2)*time.Second)
	if err != nil {
		return false
	}
	con.Close()
	return true
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
	if Exists(fileName) {
		return nil
	}
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

func Exists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

