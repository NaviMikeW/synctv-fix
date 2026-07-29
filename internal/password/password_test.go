package password

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "accepts minimum length",
			password: "123456789012",
		},
		{
			name:     "rejects empty",
			password: "",
			wantErr:  ErrEmpty,
		},
		{
			name:     "rejects short",
			password: "12345678901",
			wantErr:  ErrTooShort,
		},
		{
			name:     "rejects long",
			password: "123456789012345678901234567890123",
			wantErr:  ErrTooLong,
		},
		{
			name:     "rejects non-printable",
			password: "12345678901\n",
			wantErr:  ErrInvalidChar,
		},
		{
			name:     "rejects non-ASCII",
			password: "密码不能使用中文字符123456",
			wantErr:  ErrInvalidChar,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.password)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
