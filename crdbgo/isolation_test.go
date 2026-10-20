package crdbgo

import "testing"

func TestIsolationLevel_String_Serializable(t *testing.T) {
	got := Serializable.String()
	if got != "SERIALIZABLE" {
		t.Errorf("Serializable.String() = %q, want SERIALIZABLE", got)
	}
}

func TestIsolationLevel_String_ReadCommitted(t *testing.T) {
	got := ReadCommitted.String()
	if got != "READ COMMITTED" {
		t.Errorf("ReadCommitted.String() = %q, want READ COMMITTED", got)
	}
}

func TestIsolationLevel_String_Default(t *testing.T) {
	// An invalid IsolationLevel value should default to SERIALIZABLE.
	invalid := IsolationLevel(99)
	got := invalid.String()
	if got != "SERIALIZABLE" {
		t.Errorf("IsolationLevel(99).String() = %q, want SERIALIZABLE", got)
	}
}
