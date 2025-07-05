package config

import (
	"testing"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
)

func TestRegisterEnvName(t *testing.T) {
	if err := RegisterEnvNameForConfigKey(common.COMMON_CONFIG_KEYS_COUNT, "count"); err != nil {
		t.Fatal(err)
	}
}
