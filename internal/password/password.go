package password

import (
	"errors"
)

const (
	MinLength = 8
	MaxLength = 32
)

var (
	ErrEmpty       = errors.New("password is empty")
	ErrTooShort    = errors.New("password must be at least 8 characters")
	ErrTooLong     = errors.New("password must be at most 32 characters")
	ErrInvalidChar = errors.New("password must contain only printable ASCII characters")
)

// Validate applies the policy for newly created or changed user passwords.
// Login validation intentionally does not use this function so existing users
// with shorter passwords can still sign in and replace them.
func Validate(value string) error {
	switch {
	case value == "":
		return ErrEmpty
	case len(value) < MinLength:
		return ErrTooShort
	case len(value) > MaxLength:
		return ErrTooLong
	case !containsOnlyPrintableASCII(value):
		return ErrInvalidChar
	default:
		return nil
	}
}

func containsOnlyPrintableASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] > 0x7e {
			return false
		}
	}

	return true
}
