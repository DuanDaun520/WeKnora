package service

import (
	"errors"
	"testing"
)

func TestValidatePasswordPolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		password string
		want     error
	}{
		{name: "empty", password: "", want: ErrPasswordPolicy},
		{name: "five chars", password: "12345", want: ErrPasswordPolicy},
		{name: "whitespace only is short", password: "    ", want: ErrPasswordPolicy},
		{name: "six digits", password: "123456"},
		{name: "letters only", password: "password"},
		{name: "unicode runes count", password: "密码密码密码"},
		{name: "five unicode runes are short", password: "密码密码密", want: ErrPasswordPolicy},
		{name: "long passwords are allowed", password: "passwordpasswordpasswordpasswordpassword"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePasswordPolicy(tc.password)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidatePasswordPolicy(%q) err = %v, want %v", tc.password, err, tc.want)
			}
		})
	}
}

func TestIsPasswordPolicyError(t *testing.T) {
	t.Parallel()
	if !IsPasswordPolicyError(ErrPasswordPolicy) {
		t.Fatal("policy sentinel should match")
	}
	if IsPasswordPolicyError(ErrSamePassword) {
		t.Fatal("unrelated sentinel must not match")
	}
}
