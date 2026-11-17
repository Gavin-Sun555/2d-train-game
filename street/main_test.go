package street

import (
	"os"
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
)

func TestMain(m *testing.M) {
	locale.Current = locale.Chinese
	os.Exit(m.Run())
}
