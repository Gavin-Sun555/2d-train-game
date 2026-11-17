//go:build windows

package locale

import "golang.org/x/sys/windows"

func systemTag() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err == nil && len(langs) > 0 {
		return langs[0]
	}
	return ""
}
