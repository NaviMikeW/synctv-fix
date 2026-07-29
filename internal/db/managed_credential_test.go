package db

import (
	"bytes"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/synctv-org/synctv/internal/conf"
	"github.com/synctv-org/synctv/internal/guardian"
	"github.com/synctv-org/synctv/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	managedTestKey      = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	managedDifferentKey = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
)

func TestLocalUsersAreManagedWithoutPlaintextInDatabase(t *testing.T) {
	setupManagedCredentialTestDB(t)
	conf.Conf.Security.GuardianCredentialKey = managedTestKey

	user, err := CreateUser("child", "ChildPassword1!")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if !user.CheckPassword("ChildPassword1!") {
		t.Fatal("bcrypt login hash does not accept the created password")
	}

	var credential model.ManagedCredential
	if err = db.First(&credential, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("load managed credential: %v", err)
	}
	if bytes.Contains(credential.Ciphertext, []byte("ChildPassword1!")) {
		t.Fatal("managed credential contains the plaintext password")
	}

	stored, err := GetManagedUserCredential(user.ID)
	if err != nil {
		t.Fatalf("GetManagedUserCredential() error = %v", err)
	}
	key, err := guardian.ParseKey(managedTestKey)
	if err != nil {
		t.Fatalf("ParseKey() error = %v", err)
	}
	password, err := guardian.Decrypt(key, user.ID, stored.Ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if password != "ChildPassword1!" {
		t.Fatalf("managed password = %q, want original password", password)
	}
	if stored.KeyID == "" {
		t.Fatal("managed credential did not record its key ID")
	}
	if stored.FormatVersion != guardian.EnvelopeVersion {
		t.Fatalf(
			"managed credential version = %d, want %d",
			stored.FormatVersion,
			guardian.EnvelopeVersion,
		)
	}
}

func TestManagedPasswordChangeIsAtomic(t *testing.T) {
	setupManagedCredentialTestDB(t)
	conf.Conf.Security.GuardianCredentialKey = managedTestKey

	user, err := CreateUser("child", "ChildPassword1!")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	oldHash := append([]byte(nil), user.HashedPassword...)

	newHash, err := bcrypt.GenerateFromPassword([]byte("ChangedPassword2!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash changed password: %v", err)
	}
	conf.Conf.Security.GuardianCredentialKey = ""
	if err = SetUserPassword(user.ID, newHash, "ChangedPassword2!"); !errors.Is(
		err,
		guardian.ErrKeyNotConfigured,
	) {
		t.Fatalf("missing-key change error = %v, want ErrKeyNotConfigured", err)
	}

	var unchanged model.User
	if err = db.First(&unchanged, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("reload unchanged user: %v", err)
	}
	if !bytes.Equal(unchanged.HashedPassword, oldHash) {
		t.Fatal("bcrypt hash changed even though managed encryption failed")
	}

	conf.Conf.Security.GuardianCredentialKey = managedTestKey
	if err = SetUserPassword(user.ID, newHash, "ChangedPassword2!"); err != nil {
		t.Fatalf("SetUserPassword() error = %v", err)
	}
	stored, err := GetManagedUserCredential(user.ID)
	if err != nil {
		t.Fatalf("GetManagedUserCredential() after change error = %v", err)
	}
	key, err := guardian.ParseKey(managedTestKey)
	if err != nil {
		t.Fatalf("ParseKey() error = %v", err)
	}
	password, err := guardian.Decrypt(key, user.ID, stored.Ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() after change error = %v", err)
	}
	if password != "ChangedPassword2!" {
		t.Fatalf("managed password after change = %q", password)
	}
}

func TestPromotingManagedUserDeletesReversiblePassword(t *testing.T) {
	setupManagedCredentialTestDB(t)
	conf.Conf.Security.GuardianCredentialKey = managedTestKey

	user, err := CreateUser("child", "ChildPassword1!")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err = SetAdminRoleByID(user.ID); err != nil {
		t.Fatalf("SetAdminRoleByID() error = %v", err)
	}

	var count int64
	if err = db.Model(&model.ManagedCredential{}).
		Where("user_id = ?", user.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count managed credentials: %v", err)
	}
	if count != 0 {
		t.Fatalf("managed credentials after promotion = %d, want 0", count)
	}
	if err = BanUserByID(user.ID); err == nil {
		t.Fatal("BanUserByID(admin) unexpectedly succeeded")
	}
	if err = UnbanUserByID(user.ID); err == nil {
		t.Fatal("UnbanUserByID(admin) unexpectedly succeeded")
	}

	demotedHash, err := bcrypt.GenerateFromPassword(
		[]byte("DemotedPassword2!"),
		bcrypt.DefaultCost,
	)
	if err != nil {
		t.Fatalf("hash demoted password: %v", err)
	}
	if err = DemotePrivilegedUser(user.ID, demotedHash, "DemotedPassword2!"); err != nil {
		t.Fatalf("DemotePrivilegedUser() error = %v", err)
	}
	demotedCredential, err := GetManagedUserCredential(user.ID)
	if err != nil {
		t.Fatalf("GetManagedUserCredential(demoted) error = %v", err)
	}
	key, _ := guardian.ParseKey(managedTestKey)
	password, err := guardian.Decrypt(key, user.ID, demotedCredential.Ciphertext)
	if err != nil {
		t.Fatalf("decrypt demoted password: %v", err)
	}
	if password != "DemotedPassword2!" {
		t.Fatalf("demoted password = %q, want new managed password", password)
	}
}

func TestDeletingManagedUserDeletesReversiblePassword(t *testing.T) {
	setupManagedCredentialTestDB(t)
	conf.Conf.Security.GuardianCredentialKey = managedTestKey

	user, err := CreateUser("child", "ChildPassword1!")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err = DeleteUserByID(user.ID); err != nil {
		t.Fatalf("DeleteUserByID() error = %v", err)
	}

	var count int64
	if err = db.Model(&model.ManagedCredential{}).
		Where("user_id = ?", user.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count managed credentials: %v", err)
	}
	if count != 0 {
		t.Fatalf("managed credentials after user deletion = %d, want 0", count)
	}
}

func TestExistingAndChangedKeyStatesAreExplicit(t *testing.T) {
	setupManagedCredentialTestDB(t)
	conf.Conf.Security.GuardianCredentialKey = managedTestKey

	managed, err := CreateUser("managed-child", "ChildPassword1!")
	if err != nil {
		t.Fatalf("create managed child: %v", err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("ExistingPassword1!"), bcrypt.DefaultCost)
	existing, err := createUserWithHashedPassword(
		db,
		"existing-child",
		hash,
		WithRole(model.RoleUser),
	)
	if err != nil {
		t.Fatalf("create existing child: %v", err)
	}

	states, err := ManagedCredentialStates([]*model.User{managed, existing})
	if err != nil {
		t.Fatalf("ManagedCredentialStates() error = %v", err)
	}
	if states[managed.ID] != ManagedCredentialReady {
		t.Fatalf("managed state = %q, want ready", states[managed.ID])
	}
	if states[existing.ID] != ManagedCredentialMissing {
		t.Fatalf("existing state = %q, want missing", states[existing.ID])
	}

	conf.Conf.Security.GuardianCredentialKey = ""
	states, err = ManagedCredentialStates([]*model.User{managed, existing})
	if err != nil {
		t.Fatalf("ManagedCredentialStates() without key error = %v", err)
	}
	for _, user := range []*model.User{managed, existing} {
		if states[user.ID] != ManagedCredentialKeyNotConfigured {
			t.Fatalf("missing-key state for %s = %q, want key_not_configured", user.Username, states[user.ID])
		}
	}

	conf.Conf.Security.GuardianCredentialKey = managedDifferentKey
	states, err = ManagedCredentialStates([]*model.User{managed})
	if err != nil {
		t.Fatalf("ManagedCredentialStates() with changed key error = %v", err)
	}
	if states[managed.ID] != ManagedCredentialKeyChanged {
		t.Fatalf("changed-key state = %q, want key_changed", states[managed.ID])
	}
	if _, err = GetManagedUserCredential(managed.ID); !errors.Is(
		err,
		ErrManagedCredentialNotAvailable,
	) {
		t.Fatalf("changed-key reveal error = %v, want unavailable", err)
	}

	conf.Conf.Security.GuardianCredentialKey = managedTestKey
	if err = db.Model(&model.ManagedCredential{}).
		Where("user_id = ?", managed.ID).
		Update("format_version", guardian.EnvelopeVersion+1).Error; err != nil {
		t.Fatalf("set unsupported format version: %v", err)
	}
	states, err = ManagedCredentialStates([]*model.User{managed})
	if err != nil {
		t.Fatalf("ManagedCredentialStates() with unsupported format error = %v", err)
	}
	if states[managed.ID] != ManagedCredentialUnsupported {
		t.Fatalf("unsupported-format state = %q, want unsupported_format", states[managed.ID])
	}
}

func TestPrivilegedAndAnonymousAccountsAreNeverManaged(t *testing.T) {
	setupManagedCredentialTestDB(t)
	conf.Conf.Security.GuardianCredentialKey = ""

	for _, tc := range []struct {
		username string
		role     model.Role
		id       string
	}{
		{username: "admin", role: model.RoleAdmin},
		{username: "root", role: model.RoleRoot},
		{username: GuestUsername, role: model.RoleUser, id: GuestUserID},
	} {
		options := []CreateUserConfig{WithRole(tc.role)}
		if tc.id != "" {
			options = append(options, WithID(tc.id))
		}
		if _, err := CreateUser(tc.username, "PrivilegedPassword1!", options...); err != nil {
			t.Fatalf("CreateUser(%s) error = %v", tc.username, err)
		}
	}

	var count int64
	if err := db.Model(&model.ManagedCredential{}).Count(&count).Error; err != nil {
		t.Fatalf("count managed credentials: %v", err)
	}
	if count != 0 {
		t.Fatalf("privileged/anonymous managed credentials = %d, want 0", count)
	}
}

func TestProviderAccountsAreNeverManaged(t *testing.T) {
	setupManagedCredentialTestDB(t)
	conf.Conf.Security.GuardianCredentialKey = managedTestKey

	providerUser, err := CreateUser(
		"provider-child",
		"ProviderPassword1!",
		func(user *model.User) {
			user.RegisteredByProvider = true
		},
	)
	if err != nil {
		t.Fatalf("create provider user: %v", err)
	}

	states, err := ManagedCredentialStates([]*model.User{providerUser})
	if err != nil {
		t.Fatalf("ManagedCredentialStates() error = %v", err)
	}
	if states[providerUser.ID] != ManagedCredentialNotApplicable {
		t.Fatalf("provider state = %q, want not_applicable", states[providerUser.ID])
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte("ProviderPassword2!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash provider password: %v", err)
	}
	if err = SetUserPassword(providerUser.ID, newHash, "ProviderPassword2!"); err != nil {
		t.Fatalf("SetUserPassword(provider) error = %v", err)
	}

	var count int64
	if err = db.Model(&model.ManagedCredential{}).
		Where("user_id = ?", providerUser.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count provider credentials: %v", err)
	}
	if count != 0 {
		t.Fatalf("provider managed credentials = %d, want 0", count)
	}
	if _, err = GetManagedUserCredential(providerUser.ID); !errors.Is(
		err,
		ErrUserNotManaged,
	) {
		t.Fatalf("provider reveal error = %v, want ErrUserNotManaged", err)
	}
}

func TestMissingKeyPreventsCreatingManagedUser(t *testing.T) {
	setupManagedCredentialTestDB(t)
	conf.Conf.Security.GuardianCredentialKey = ""

	if _, err := CreateUser("child", "ChildPassword1!"); !errors.Is(
		err,
		guardian.ErrKeyNotConfigured,
	) {
		t.Fatalf("CreateUser() error = %v, want ErrKeyNotConfigured", err)
	}

	var count int64
	if err := db.Model(&model.User{}).
		Where("username = ?", "child").
		Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("users created without guardian key = %d, want 0", count)
	}
}

func setupManagedCredentialTestDB(t *testing.T) {
	t.Helper()

	previousDB := db
	previousDBType := dbType
	previousConfig := conf.Conf

	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err = testDB.AutoMigrate(&model.User{}, &model.ManagedCredential{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	db = testDB
	dbType = conf.DatabaseTypeSqlite3
	conf.Conf = conf.DefaultConfig()

	t.Cleanup(func() {
		sqlDB, sqlErr := testDB.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
		db = previousDB
		dbType = previousDBType
		conf.Conf = previousConfig
	})
}
