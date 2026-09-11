package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Error("correct password should verify")
	}
	if VerifyPassword("wrong password", hash) {
		t.Error("wrong password must not verify")
	}
	if VerifyPassword("anything", "not-a-valid-hash") {
		t.Error("malformed hash must not verify")
	}
	if VerifyPassword("anything", dummyHash) {
		t.Error("dummy hash must never verify")
	}
}

func TestValidateEmail(t *testing.T) {
	valid := []string{"a@b.co", "user.name+tag@example.com"}
	for _, e := range valid {
		if !validateEmail(e) {
			t.Errorf("validateEmail(%q) = false, want true", e)
		}
	}
	invalid := []string{"", "a", "a@b", "@b.com", "a@.com", "a b@c.com"}
	for _, e := range invalid {
		if validateEmail(e) {
			t.Errorf("validateEmail(%q) = true, want false", e)
		}
	}
}
