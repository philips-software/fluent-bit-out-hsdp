package hsdp

import (
	"testing"
)

func TestCamelCaeToUnderscore(t *testing.T) {
	out := CamelCaseToUnderscore("IngestorHost")
	if out != "ingestor_host" {
		t.Errorf("Unexpected conversion: %s", out)
	}
}
