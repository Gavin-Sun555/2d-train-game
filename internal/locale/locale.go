// Package locale holds the game's display language: Simplified Chinese or
// English. Every piece of interface text is written in both, side by side,
// with T(zh, en). The language defaults to the system's: Chinese if the
// first preferred language is Chinese, English otherwise.
package locale

import (
	"os"
	"strings"
	"unicode"
)

type Language int

const (
	English Language = iota
	Chinese
)

// Current is the language in use.
var Current = English

// Code is the language's short name, as stored in settings.
func (l Language) Code() string {
	if l == Chinese {
		return "zh"
	}
	return "en"
}

// Name is the language's own name.
func (l Language) Name() string {
	if l == Chinese {
		return "中文"
	}
	return "English"
}

// FromCode reads a language code ("zh", "en", "zh-Hans", ...).
func FromCode(code string) (Language, bool) {
	switch base(code) {
	case "zh":
		return Chinese, true
	case "en":
		return English, true
	}
	return English, false
}

// T is the text in the current language.
func T(zh, en string) string {
	if Current == Chinese {
		return zh
	}
	return en
}

// L is the list in the current language.
func L(zh, en []string) []string {
	if Current == Chinese {
		return zh
	}
	return en
}

// System is the system's language: Chinese if its first preferred
// language is Chinese, English otherwise.
func System() Language {
	tag := systemTag()
	if tag == "" {
		tag = envTag(os.Getenv)
	}
	if base(tag) == "zh" {
		return Chinese
	}
	return English
}

func base(tag string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(tag)), func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == '@' || r == ':'
	})
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func envTag(getenv func(string) string) string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		if v := strings.TrimSpace(getenv(key)); v != "" && v != "C" && v != "POSIX" {
			return strings.SplitN(v, ":", 2)[0]
		}
	}
	return ""
}

// firstAppleLanguage reads the first entry of `defaults read -g
// AppleLanguages` (a quoted, comma-separated list in parentheses). Only the
// first preferred language counts.
func firstAppleLanguage(out string) string {
	tags := strings.FieldsFunc(out, func(r rune) bool {
		return unicode.IsSpace(r) || r == '(' || r == ')' || r == ',' || r == '"'
	})
	if len(tags) > 0 {
		return tags[0]
	}
	return ""
}
