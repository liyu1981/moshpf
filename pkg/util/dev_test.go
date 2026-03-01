package util

import (
	"os"
	"testing"
)

func TestIsDev(t *testing.T) {
	orig := os.Getenv("APP_ENV")
	defer os.Setenv("APP_ENV", orig)

	os.Setenv("APP_ENV", "dev")
	if !IsDev() {
		t.Error("Expected IsDev() to be true")
	}

	os.Setenv("APP_ENV", "prod")
	if IsDev() {
		t.Error("Expected IsDev() to be false")
	}
}
