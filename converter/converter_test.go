package converter

import "testing"

func TestConvertString(t *testing.T) {
	got, err := Convert[string](42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "42" {
		t.Fatalf("got %q, want %q", got, "42")
	}
}

func TestConvertInt(t *testing.T) {
	got, err := Convert[int]("123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 123 {
		t.Fatalf("got %d, want %d", got, 123)
	}
}

func TestConvertFloat64(t *testing.T) {
	got, err := Convert[float64]("3.14")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3.14 {
		t.Fatalf("got %v, want %v", got, 3.14)
	}
}

func TestConvertBool(t *testing.T) {
	got, err := Convert[bool]("true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatal("got false, want true")
	}
}

func TestConvertInvalid(t *testing.T) {
	_, err := Convert[int]("abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
