package game

import (
	"os"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
)

// The tests check Chinese text; the English is checked in locale_test.go.
func TestMain(m *testing.M) {
	locale.Current = locale.Chinese
	os.Exit(m.Run())
}
