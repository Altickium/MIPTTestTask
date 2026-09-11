package qrsvg

import (
	"bytes"
	"testing"
)

func TestGenerateSVG(t *testing.T) {
	g := Generator{}
	first, err := g.GenerateSVG("https://example.test/p/poll-a")
	if err != nil {
		t.Fatal(err)
	}
	again, err := g.GenerateSVG("https://example.test/p/poll-a")
	if err != nil {
		t.Fatal(err)
	}
	other, err := g.GenerateSVG("https://example.test/p/poll-b")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(first, []byte(`<?xml version="1.0"`)) || !bytes.Contains(first, []byte(`<svg`)) {
		t.Fatalf("not SVG: %q", first[:min(80, len(first))])
	}
	if !bytes.Equal(first, again) {
		t.Fatal("same content must produce deterministic SVG")
	}
	if bytes.Equal(first, other) {
		t.Fatal("different URLs must produce different QR codes")
	}
	if bytes.Contains(first, []byte("example.test")) {
		t.Fatal("encoded URL must not be emitted as SVG text")
	}
}
