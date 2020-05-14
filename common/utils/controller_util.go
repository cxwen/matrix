package utils

import (
	"github.com/wonderivan/logger"
	"k8s.io/apimachinery/pkg/api/errors"
)

func IgnoreNotFound(err error) error {
	if errors.IsNotFound(err) {
		logger.Info(err)
		return nil
	}
	return err
}

func IgnoreAlreadyExist(err error) error {
	if errors.IsAlreadyExists(err) {
		logger.Info(err)
		return nil
	}
	return err
}

func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
