package db

import "testing"

func TestRegisterCachePrefixString(t *testing.T) {
	if err := RegisterCachePrefixString(CACHE_KEY_PREFIXES_COUNT, "count"); err != nil {
		t.Fatal(err)
	}
}
