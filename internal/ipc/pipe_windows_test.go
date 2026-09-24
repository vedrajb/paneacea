package ipc

import "testing"

func TestDialAddressUsesEnvironmentOverride(t *testing.T) {
	const expected = `\\.\pipe\paneacea-test`
	t.Setenv("PANEACEA_PIPE", expected)
	actual, err := dialAddress()
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("dial address = %q, want %q", actual, expected)
	}
}

func TestAddressIgnoresEnvironmentOverride(t *testing.T) {
	const override = `\\.\pipe\paneacea-test`
	t.Setenv("PANEACEA_PIPE", override)
	actual, err := Address()
	if err != nil {
		t.Fatal(err)
	}
	if actual == override {
		t.Fatalf("installation address used PANEACEA_PIPE: %q", actual)
	}
}

func TestLocalDialAddressIgnoresEnvironmentOverride(t *testing.T) {
	const override = `\\.\pipe\paneacea-test`
	t.Setenv("PANEACEA_PIPE", override)
	actual, err := localDialAddress()
	if err != nil {
		t.Fatal(err)
	}
	if actual == override {
		t.Fatalf("local dial address used PANEACEA_PIPE: %q", actual)
	}
}

func TestPipeSuffixInstallationIdentity(t *testing.T) {
	first := pipeSuffix(`C:\Apps\Paneacea\build\bin`)
	same := pipeSuffix(`c:\apps\paneacea\build\bin\.`)
	different := pipeSuffix(`C:\Apps\Paneacea\paneacea-portable`)
	if first != same {
		t.Fatalf("same installation directories have different suffixes: %q and %q", first, same)
	}
	if first == different {
		t.Fatalf("different installation directories have the same suffix: %q", first)
	}
}
