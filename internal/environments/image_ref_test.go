package environments

import "testing"

func TestParseImageRef(t *testing.T) {
	arn, version, ok := ParseImageRef("lambda:arn:aws:lambda:us-east-1:1:microvm-image:agent:1.0")
	if !ok {
		t.Fatal("expected valid ref")
	}
	if arn != "arn:aws:lambda:us-east-1:1:microvm-image:agent" || version != "1.0" {
		t.Fatalf("arn=%q version=%q", arn, version)
	}
	if _, _, ok := ParseImageRef("local:"); ok {
		t.Fatal("local ref should not parse as lambda image")
	}
}
