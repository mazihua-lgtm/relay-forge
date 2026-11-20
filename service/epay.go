package service

import (
	"github.com/mazihua-lgtm/relay-forge/setting/operation_setting"
	"github.com/mazihua-lgtm/relay-forge/setting/system_setting"
)

func GetCallbackAddress() string {
	if operation_setting.CustomCallbackAddress == "" {
		return system_setting.ServerAddress
	}
	return operation_setting.CustomCallbackAddress
}
