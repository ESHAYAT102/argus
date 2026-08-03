package api

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPresentDeviceAuthorizationCopiesThenOpens(t *testing.T) {
	var output bytes.Buffer
	copied, opened := "", ""
	client := &HTTPClient{
		input: strings.NewReader("\n"), output: &output,
		copy: func(value string) error { copied = value; return nil },
		open: func(value string) error { opened = value; return nil },
	}
	device := deviceAuth{UserCode: "EACA-64BA", VerificationURI: "https://github.com/login/device"}
	if err := client.presentDeviceAuthorization(device); err != nil {
		t.Fatal(err)
	}
	if copied != device.UserCode {
		t.Fatalf("copied %q", copied)
	}
	if opened != device.VerificationURI {
		t.Fatalf("opened %q", opened)
	}
	want := "Your one-time code: EACA-64BA\nPress Enter to open https://github.com/login/device in your browser...\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestPresentDeviceAuthorizationFallsBackGracefully(t *testing.T) {
	var output bytes.Buffer
	client := &HTTPClient{
		input: strings.NewReader("\n"), output: &output,
		copy: func(string) error { return errors.New("unavailable") },
		open: func(string) error { return errors.New("unavailable") },
	}
	device := deviceAuth{UserCode: "EACA-64BA", VerificationURI: "https://github.com/login/device"}
	if err := client.presentDeviceAuthorization(device); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "copy it from below") || !strings.Contains(output.String(), "Open https://github.com/login/device manually") {
		t.Fatalf("missing fallback guidance: %q", output.String())
	}
}

func TestPresentDeviceAuthorizationRequiresEnter(t *testing.T) {
	client := &HTTPClient{input: strings.NewReader(""), output: &bytes.Buffer{}, copy: func(string) error { return nil }, open: func(string) error { t.Fatal("browser should not open"); return nil }}
	err := client.presentDeviceAuthorization(deviceAuth{UserCode: "CODE", VerificationURI: "https://github.com/login/device"})
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("unexpected error: %v", err)
	}
}
