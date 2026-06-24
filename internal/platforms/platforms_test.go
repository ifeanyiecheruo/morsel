package platforms_test

import (
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/platforms"
)

func TestCreateLocalByName(t *testing.T) {
	plat, err := platforms.Create("local", nil)
	if err != nil {
		t.Fatalf("Create(local): unexpected error: %v", err)
	}
	if plat == nil {
		t.Error("Create(local): returned nil platform")
	}
}

func TestCreateUnknownReturnsError(t *testing.T) {
	plat, err := platforms.Create("aws", nil)
	if err == nil {
		t.Error("Create(aws): expected error, got nil")
	}
	if plat != nil {
		t.Error("Create(aws): expected nil platform on error")
	}
}
