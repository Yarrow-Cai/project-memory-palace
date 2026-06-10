package i18n

import "testing"

func TestEnglishTranslation(t *testing.T) {
	SetLanguage("en")
	if T("app_title") != "Project Memory Palace" {
		t.Fatalf("expected 'Project Memory Palace', got '%s'", T("app_title"))
	}
}

func TestChineseTranslation(t *testing.T) {
	SetLanguage("zh")
	if T("browse") == "Browse" {
		t.Fatal("expected Chinese translation for browse")
	}
}

func TestFallback(t *testing.T) {
	SetLanguage("en")
	if T("no_such_key") != "no_such_key" {
		t.Fatal("expected key itself as fallback")
	}
}

func TestSetUnknownLanguage(t *testing.T) {
	SetLanguage("en")
	SetLanguage("fr")
	if GetLanguage() != "en" {
		t.Fatal("unknown language should not change current")
	}
}

func TestDefaultLanguage(t *testing.T) {
	if GetLanguage() != "en" {
		t.Fatal("default should be en")
	}
}
