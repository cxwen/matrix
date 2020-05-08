package controllers

import (
	"github.com/wonderivan/logger"
	"k8s.io/apimachinery/pkg/api/errors"
)

func ignoreNotFound(err error) error {
	if errors.IsNotFound(err) {
		logger.Info(err)
		return nil
	}
	return err
}

func ignoreAlreadyExist(err error) error {
	if errors.IsAlreadyExists(err) {
		logger.Info(err)
		return nil
	}
	return err
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
