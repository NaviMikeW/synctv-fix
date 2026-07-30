package db

import (
	"errors"
	"fmt"

	"github.com/synctv-org/synctv/internal/conf"
	"github.com/synctv-org/synctv/internal/guardian"
	"github.com/synctv-org/synctv/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ManagedCredentialState string

const (
	ManagedCredentialNotApplicable    ManagedCredentialState = "not_applicable"
	ManagedCredentialMissing          ManagedCredentialState = "missing"
	ManagedCredentialReady            ManagedCredentialState = "ready"
	ManagedCredentialKeyChanged       ManagedCredentialState = "key_changed"
	ManagedCredentialUnsupported      ManagedCredentialState = "unsupported_format"
	ManagedCredentialKeyNotConfigured ManagedCredentialState = "key_not_configured"
)

var (
	ErrManagedCredentialNotAvailable = errors.New("managed password is not available")
	ErrUserNotManaged                = errors.New("this account is not a managed local user")
)

func IsManagedUser(user *model.User) bool {
	if user == nil || user.ID == GuestUserID || user.RegisteredByProvider {
		return false
	}
	switch user.Role {
	case model.RoleBanned, model.RolePending, model.RoleUser:
		return true
	default:
		return false
	}
}

func activeGuardianKey() (guardian.Key, error) {
	return guardian.ParseKey(conf.Conf.Security.GuardianCredentialKey)
}

func newManagedCredential(userID, password string) (*model.ManagedCredential, error) {
	key, err := activeGuardianKey()
	if err != nil {
		return nil, err
	}
	ciphertext, err := guardian.Encrypt(key, userID, password)
	if err != nil {
		return nil, err
	}
	return &model.ManagedCredential{
		UserID:        userID,
		FormatVersion: guardian.EnvelopeVersion,
		KeyID:         key.ID(),
		Ciphertext:    ciphertext,
	}, nil
}

func saveManagedCredential(tx *gorm.DB, credential *model.ManagedCredential) error {
	if err := tx.Save(credential).Error; err != nil {
		return fmt.Errorf("save managed password: %w", err)
	}
	return nil
}

func SetUserPassword(userID string, hashedPassword []byte, password string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "role", "registered_by_provider").
			Where("id = ?", userID).
			First(&user).Error; err != nil {
			return HandleNotFound(err, ErrUserNotFound)
		}

		result := tx.Model(&model.User{}).
			Where("id = ?", userID).
			Update("hashed_password", hashedPassword)
		if updateErr := HandleUpdateResult(result, ErrUserNotFound); updateErr != nil {
			return updateErr
		}

		if !IsManagedUser(&user) {
			return deleteManagedCredential(tx, userID)
		}

		credential, credentialErr := newManagedCredential(userID, password)
		if credentialErr != nil {
			return credentialErr
		}
		return saveManagedCredential(tx, credential)
	})
}

func GetManagedUserCredential(userID string) (model.ManagedCredential, error) {
	activeKey, err := activeGuardianKey()
	if err != nil {
		return model.ManagedCredential{}, err
	}

	var user model.User
	if err = db.Select("id", "role", "registered_by_provider").
		Where("id = ?", userID).
		First(&user).Error; err != nil {
		return model.ManagedCredential{}, HandleNotFound(err, ErrUserNotFound)
	}
	if !IsManagedUser(&user) {
		return model.ManagedCredential{}, ErrUserNotManaged
	}

	var credential model.ManagedCredential
	if err = db.Where("user_id = ?", userID).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ManagedCredential{}, ErrManagedCredentialNotAvailable
		}
		return model.ManagedCredential{}, fmt.Errorf("load managed password: %w", err)
	}
	if credential.KeyID != activeKey.ID() {
		return model.ManagedCredential{}, ErrManagedCredentialNotAvailable
	}
	if credential.FormatVersion != guardian.EnvelopeVersion {
		return model.ManagedCredential{}, ErrManagedCredentialNotAvailable
	}
	return credential, nil
}

func ManagedCredentialStates(users []*model.User) (map[string]ManagedCredentialState, error) {
	states := make(map[string]ManagedCredentialState, len(users))
	ids := make([]string, 0, len(users))
	activeKey, keyErr := activeGuardianKey()
	for _, user := range users {
		if !IsManagedUser(user) {
			states[user.ID] = ManagedCredentialNotApplicable
			continue
		}
		ids = append(ids, user.ID)
		if keyErr != nil {
			states[user.ID] = ManagedCredentialKeyNotConfigured
		} else {
			states[user.ID] = ManagedCredentialMissing
		}
	}
	if len(ids) == 0 {
		return states, nil
	}

	var credentials []model.ManagedCredential
	if err := db.Select("user_id", "format_version", "key_id").
		Where("user_id IN ?", ids).
		Find(&credentials).Error; err != nil {
		return nil, fmt.Errorf("load managed-password states: %w", err)
	}

	for _, credential := range credentials {
		switch {
		case keyErr != nil:
			states[credential.UserID] = ManagedCredentialKeyNotConfigured
		case credential.FormatVersion != guardian.EnvelopeVersion:
			states[credential.UserID] = ManagedCredentialUnsupported
		case credential.KeyID != activeKey.ID():
			states[credential.UserID] = ManagedCredentialKeyChanged
		default:
			states[credential.UserID] = ManagedCredentialReady
		}
	}
	return states, nil
}

func deleteManagedCredential(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&model.ManagedCredential{}).Error; err != nil {
		return fmt.Errorf("delete managed password: %w", err)
	}
	return nil
}

func setPrivilegedRoleByID(userID string, role model.Role) error {
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.User{}).Where("id = ?", userID).Update("role", role)
		if err := HandleUpdateResult(result, ErrUserNotFound); err != nil {
			return err
		}
		return deleteManagedCredential(tx, userID)
	})
}

func DemotePrivilegedUser(userID string, hashedPassword []byte, password string) error {
	credential, err := newManagedCredential(userID, password)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "role", "registered_by_provider").
			Where("id = ?", userID).
			First(&user).Error; err != nil {
			return HandleNotFound(err, ErrUserNotFound)
		}
		if user.RegisteredByProvider ||
			(user.Role != model.RoleAdmin && user.Role != model.RoleRoot) {
			return errors.New("only a local administrator or root can be demoted")
		}

		result := tx.Model(&model.User{}).
			Where("id = ?", userID).
			Updates(map[string]any{
				"role":            model.RoleUser,
				"hashed_password": hashedPassword,
			})
		if updateErr := HandleUpdateResult(result, ErrUserNotFound); updateErr != nil {
			return updateErr
		}
		return saveManagedCredential(tx, credential)
	})
}
