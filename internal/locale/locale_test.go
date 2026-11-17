package locale

import "testing"

func TestLanguageTags(t *testing.T) {
	for tag, want := range map[string]string{
		"zh-Hans-CN": "zh", "zh_CN.UTF-8": "zh", "zh-Hant-TW": "zh", "en-US": "en", "fr_FR.UTF-8": "en", "": "en",
	} {
		got := English
		if base(tag) == "zh" {
			got = Chinese
		}
		if got.Code() != want {
			t.Errorf("%q → %s, want %s", tag, got.Code(), want)
		}
	}
	if got := firstAppleLanguage("(\n    \"en-US\",\n    \"zh-Hans-CN\"\n)\n"); got != "en-US" {
		t.Errorf("first Apple language %q", got)
	}
	env := map[string]string{"LANG": "zh_CN.UTF-8", "LC_ALL": "C"}
	if got := envTag(func(k string) string { return env[k] }); got != "zh_CN.UTF-8" {
		t.Errorf("env tag %q", got)
	}
	Current = Chinese
	if T("中", "en") != "中" || L([]string{"a"}, []string{"b"})[0] != "a" {
		t.Error("Chinese text")
	}
	Current = English
	if T("中", "en") != "en" {
		t.Error("English text")
	}
}
