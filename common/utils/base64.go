package utils

import (
	"encoding/base64"
)

func Base64Encode(bytes []byte) string {
	return base64.StdEncoding.EncodeToString(bytes)
}

func Base64Decode(str string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(str)
}
