package main

import (
	"encoding/pem"
	"testing"
)

func TestGenerateECHKeyPairOutputOrder(t *testing.T) {
	output, err := generateECHKeyPairOutput("example.com")
	if err != nil {
		t.Fatal(err)
	}

	keyBlock, rest := pem.Decode([]byte(output))
	if keyBlock == nil {
		t.Fatal("missing first PEM block")
	}
	if keyBlock.Type != "ECH KEYS" {
		t.Fatalf("unexpected first PEM block: %s", keyBlock.Type)
	}

	configBlock, rest := pem.Decode(rest)
	if configBlock == nil {
		t.Fatal("missing second PEM block")
	}
	if configBlock.Type != "ECH CONFIGS" {
		t.Fatalf("unexpected second PEM block: %s", configBlock.Type)
	}
	if len(rest) != 0 {
		t.Fatalf("unexpected trailing output: %q", rest)
	}
}
