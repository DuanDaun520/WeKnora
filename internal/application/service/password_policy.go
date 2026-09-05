package service

import (
	"errors"
	"unicode/utf8"
)

// IsPasswordPolicyError reports whether err is a documented password-policy
// failure so HTTP handlers can translate it to a 400 without exposing
// bcrypt or persistence errors.
func IsPasswordPolicyError(err error) bool {
	return errors.Is(err, ErrPasswordPolicy)
}

// ValidatePasswordPolicy keeps registration, self-service rotation and
// administrative password resets aligned with the SPA form. The only
// requirement is a minimum length of 6 characters; no character-class
// rules apply. Password bytes are never logged or included in the
// returned error.
func ValidatePasswordPolicy(password string) error {
	if utf8.RuneCountInString(password) < 6 {
		return ErrPasswordPolicy
	}
	return nil
}
