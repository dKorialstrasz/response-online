package application

import "testing"

func FuzzNormalizeRoomCode(fuzz *testing.F) {
	fuzz.Add("ABCDEFGHIJKLM")
	fuzz.Add("2345672345672")
	fuzz.Add("../secret-value")
	fuzz.Add("АБВГДЕЖЗИЙКЛМ")
	fuzz.Fuzz(func(t *testing.T, input string) {
		code, valid := normalizeRoomCode(input, DefaultRoomCodeLength)
		if !valid {
			if code != "" {
				t.Fatalf("invalid code returned normalized material %q", code)
			}
			return
		}
		if len(code) != DefaultRoomCodeLength {
			t.Fatalf("valid code length = %d", len(code))
		}
		for _, character := range code {
			if (character < 'A' || character > 'Z') && (character < '2' || character > '7') {
				t.Fatalf("valid code contains %q", character)
			}
		}
	})
}
