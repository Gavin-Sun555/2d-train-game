//go:build !darwin && !windows

package locale

func systemTag() string { return "" }
