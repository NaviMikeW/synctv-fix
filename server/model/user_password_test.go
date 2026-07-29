package model

import (
	"errors"
	"testing"

	dbModel "github.com/synctv-org/synctv/internal/model"
)

func TestNewUserPasswordRequestsRequireTwelveCharacters(t *testing.T) {
	const shortPassword = "12345678901"

	tests := []struct {
		name     string
		validate func() error
	}{
		{
			name: "user changes password",
			validate: func() error {
				return (&SetUserPasswordReq{Password: shortPassword}).Validate()
			},
		},
		{
			name: "password signup",
			validate: func() error {
				return (&UserSignupPasswordReq{
					Username: "new-user",
					Password: shortPassword,
				}).Validate()
			},
		},
		{
			name: "email signup",
			validate: func() error {
				return (&UserSignupEmailReq{
					UserBindEmailReq: UserBindEmailReq{
						Email:   "user@example.com",
						Captcha: "123456",
					},
					Password: shortPassword,
				}).Validate()
			},
		},
		{
			name: "email password reset",
			validate: func() error {
				return (&UserRetrievePasswordEmailReq{
					Email:    "user@example.com",
					Captcha:  "123456",
					Password: shortPassword,
				}).Validate()
			},
		},
		{
			name: "admin creates user",
			validate: func() error {
				return (&AddUserReq{
					Username: "new-user",
					Password: shortPassword,
					Role:     dbModel.RoleUser,
				}).Validate()
			},
		},
		{
			name: "admin resets password",
			validate: func() error {
				return (&AdminUserPasswordReq{
					ID:       "user-id",
					Password: shortPassword,
				}).Validate()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.validate(); !errors.Is(err, ErrPasswordTooShort) {
				t.Fatalf("Validate() error = %v, want %v", err, ErrPasswordTooShort)
			}
		})
	}
}

func TestNewUserPasswordAcceptsTwelveCharacters(t *testing.T) {
	req := SetUserPasswordReq{Password: "123456789012"}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoginStillAcceptsLegacyShortPassword(t *testing.T) {
	req := LoginUserReq{
		Username: "root",
		Password: "root",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("legacy login validation error = %v", err)
	}
}
