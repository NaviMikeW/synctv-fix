package db

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	log "github.com/sirupsen/logrus"
	"github.com/synctv-org/synctv/cmd/flags"
	"github.com/synctv-org/synctv/internal/conf"
	"github.com/synctv-org/synctv/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestInitRootUserCreatesRandomPasswordOnce(t *testing.T) {
	setupRootTestDB(t)

	var output bytes.Buffer
	logger := log.StandardLogger()
	previousOutput := logger.Out
	logger.SetOutput(&output)
	t.Cleanup(func() {
		logger.SetOutput(previousOutput)
	})

	if err := initRootUser(); err != nil {
		t.Fatalf("initRootUser() error = %v", err)
	}

	root := getTestRoot(t)
	if root.CheckPassword(model.LegacyDefaultRootPassword) {
		t.Fatal("new root user still accepts the legacy default password")
	}

	passwordFile := getTestInitialRootPasswordFile(t, root)
	passwordBytes, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatalf("read initial password file: %v", err)
	}
	generatedPassword := strings.TrimSpace(string(passwordBytes))
	if !root.CheckPassword(generatedPassword) {
		t.Fatal("saved password does not authenticate the created root user")
	}
	if !RootPasswordNeedsChange(root) {
		t.Fatal("generated initial password was not marked for replacement")
	}
	if strings.Contains(output.String(), generatedPassword) {
		t.Fatal("generated password was written to regular logs")
	}
	if !strings.Contains(output.String(), passwordFile) {
		t.Fatalf("initial password file path was not logged: %s", output.String())
	}
	info, err := os.Stat(passwordFile)
	if err != nil {
		t.Fatalf("stat initial password file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("initial password file mode = %o, want 600", got)
	}

	output.Reset()
	if err := initRootUser(); err != nil {
		t.Fatalf("second initRootUser() error = %v", err)
	}
	passwordBytesAfterRestart, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatalf("read initial password file after restart: %v", err)
	}
	if string(passwordBytesAfterRestart) != string(passwordBytes) {
		t.Fatal("initial password file changed for an existing database")
	}
}

func TestInitRootUserCreatesWithConfiguredPassword(t *testing.T) {
	setupRootTestDB(t)
	conf.Conf.Security.InitialRootPassword = "Configured1!"

	if err := initRootUser(); err != nil {
		t.Fatalf("initRootUser() error = %v", err)
	}

	if !getTestRoot(t).CheckPassword("Configured1!") {
		t.Fatal("root user does not accept the configured initial password")
	}
	if RootPasswordNeedsChange(getTestRoot(t)) {
		t.Fatal("configured initial password was incorrectly marked for replacement")
	}
}

func TestInitRootUserRotatesLegacyPasswordWithoutConfiguration(t *testing.T) {
	setupRootTestDB(t)
	createTestRoot(t, model.LegacyDefaultRootPassword)

	if err := initRootUser(); err != nil {
		t.Fatalf("initRootUser() error = %v", err)
	}

	root := getTestRoot(t)
	if root.CheckPassword(model.LegacyDefaultRootPassword) {
		t.Fatal("legacy root password still works after automatic rotation")
	}
	passwordFile := getTestInitialRootPasswordFile(t, root)
	passwordBytes, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatalf("read replacement password file: %v", err)
	}
	replacementPassword := strings.TrimSpace(string(passwordBytes))
	if !root.CheckPassword(replacementPassword) {
		t.Fatal("saved replacement password does not authenticate the legacy root user")
	}
	if !RootPasswordNeedsChange(root) {
		t.Fatal("generated replacement password was not marked for replacement")
	}
}

func TestInitRootUserRotatesLegacyPasswordWhenConfigured(t *testing.T) {
	setupRootTestDB(t)
	createTestRoot(t, model.LegacyDefaultRootPassword)
	conf.Conf.Security.InitialRootPassword = "Replacement1!"

	if err := initRootUser(); err != nil {
		t.Fatalf("initRootUser() error = %v", err)
	}

	root := getTestRoot(t)
	if root.CheckPassword(model.LegacyDefaultRootPassword) {
		t.Fatal("legacy root password still works after explicit rotation")
	}
	if !root.CheckPassword("Replacement1!") {
		t.Fatal("configured replacement password does not work")
	}
	if RootPasswordNeedsChange(root) {
		t.Fatal("configured replacement password was incorrectly marked for replacement")
	}
}

func TestInitRootUserNeverOverwritesCustomPassword(t *testing.T) {
	setupRootTestDB(t)
	createTestRoot(t, "ExistingCustom1!")
	conf.Conf.Security.InitialRootPassword = "Replacement1!"

	if err := initRootUser(); err != nil {
		t.Fatalf("initRootUser() error = %v", err)
	}

	root := getTestRoot(t)
	if !root.CheckPassword("ExistingCustom1!") {
		t.Fatal("existing custom root password was overwritten")
	}
	if root.CheckPassword("Replacement1!") {
		t.Fatal("configured initial password replaced a custom root password")
	}
}

func TestInitialRootPasswordFilesAreBoundToUserID(t *testing.T) {
	setupRootTestDB(t)
	firstRoot := createTestRootNamed(t, "root", "FirstRootStrong1!")
	secondRoot := createTestRootNamed(t, "backup-root", "SecondRootStrong1!")

	firstPasswordFile, err := writeInitialRootPasswordFile(
		firstRoot.ID,
		"FirstRootStrong1!",
	)
	if err != nil {
		t.Fatalf("write first root password file: %v", err)
	}

	if err = RemoveInitialRootPasswordFile(secondRoot.ID); err != nil {
		t.Fatalf("remove second root password file: %v", err)
	}
	if _, err = os.Stat(firstPasswordFile); err != nil {
		t.Fatalf("second root removed first root password file: %v", err)
	}
	if !RootPasswordNeedsChange(firstRoot) {
		t.Fatal("first root did not receive its initial-password reminder")
	}
	if RootPasswordNeedsChange(secondRoot) {
		t.Fatal("second root received another user's initial-password reminder")
	}

	if err = initRootUser(); err != nil {
		t.Fatalf("initRootUser() with multiple roots error = %v", err)
	}
	if _, err = os.Stat(firstPasswordFile); err != nil {
		t.Fatalf("restart removed the matching root password file: %v", err)
	}
}

func TestInitRootUserRotatesEveryLegacyRootSeparately(t *testing.T) {
	setupRootTestDB(t)
	firstRoot := createTestRootNamed(t, "root", model.LegacyDefaultRootPassword)
	secondRoot := createTestRootNamed(t, "backup-root", model.LegacyDefaultRootPassword)

	if err := initRootUser(); err != nil {
		t.Fatalf("initRootUser() error = %v", err)
	}

	for _, root := range []*model.User{firstRoot, secondRoot} {
		var reloaded model.User
		if err := db.First(&reloaded, "id = ?", root.ID).Error; err != nil {
			t.Fatalf("reload root %s: %v", root.Username, err)
		}
		if reloaded.CheckPassword(model.LegacyDefaultRootPassword) {
			t.Fatalf("legacy password still works for %s", root.Username)
		}

		passwordBytes, err := os.ReadFile(
			getTestInitialRootPasswordFile(t, &reloaded),
		)
		if err != nil {
			t.Fatalf("read replacement password for %s: %v", root.Username, err)
		}
		if !reloaded.CheckPassword(strings.TrimSpace(string(passwordBytes))) {
			t.Fatalf("replacement password does not work for %s", root.Username)
		}
	}
}

func TestResetRootPasswordToRecoveryFile(t *testing.T) {
	setupRootTestDB(t)
	firstRoot := createTestRootNamed(t, "root", "ExistingCustom1!")
	secondRoot := createTestRootNamed(t, "backup-root", "SecondRootStrong1!")

	path, err := ResetRootPasswordToRecoveryFile(firstRoot.ID)
	if err != nil {
		t.Fatalf("ResetRootPasswordToRecoveryFile() error = %v", err)
	}
	passwordBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recovery password file: %v", err)
	}
	recoveryPassword := strings.TrimSpace(string(passwordBytes))

	var reloadedFirst model.User
	if err = db.First(&reloadedFirst, "id = ?", firstRoot.ID).Error; err != nil {
		t.Fatalf("reload first root: %v", err)
	}
	if reloadedFirst.CheckPassword("ExistingCustom1!") {
		t.Fatal("old root password still works after explicit recovery reset")
	}
	if !reloadedFirst.CheckPassword(recoveryPassword) {
		t.Fatal("recovery-file password does not authenticate the reset root")
	}
	if !RootPasswordNeedsChange(&reloadedFirst) {
		t.Fatal("reset root was not marked for password replacement")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat recovery password file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("recovery password file mode = %o, want 600", got)
	}

	if _, err = ResetRootPasswordToRecoveryFile(firstRoot.ID); err == nil {
		t.Fatal("second reset unexpectedly replaced an existing recovery file")
	}
	var reloadedAfterSecondAttempt model.User
	if err = db.First(
		&reloadedAfterSecondAttempt,
		"id = ?",
		firstRoot.ID,
	).Error; err != nil {
		t.Fatalf("reload first root after second reset: %v", err)
	}
	if !reloadedAfterSecondAttempt.CheckPassword(recoveryPassword) {
		t.Fatal("second reset attempt changed the active recovery password")
	}

	var reloadedSecond model.User
	if err = db.First(&reloadedSecond, "id = ?", secondRoot.ID).Error; err != nil {
		t.Fatalf("reload second root: %v", err)
	}
	if !reloadedSecond.CheckPassword("SecondRootStrong1!") {
		t.Fatal("resetting one root changed another root password")
	}
}

func TestResetRootPasswordRejectsNonRoot(t *testing.T) {
	setupRootTestDB(t)
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte("RegularUser1!"),
		bcrypt.DefaultCost,
	)
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	user, err := CreateUserWithHashedPassword(
		"regular-user",
		hashedPassword,
		WithRole(model.RoleUser),
	)
	if err != nil {
		t.Fatalf("create regular user: %v", err)
	}

	if _, err = ResetRootPasswordToRecoveryFile(user.ID); err == nil {
		t.Fatal("ResetRootPasswordToRecoveryFile() accepted a non-root user")
	}
}

func setupRootTestDB(t *testing.T) {
	t.Helper()

	previousDB := db
	previousDBType := dbType
	previousConfig := conf.Conf
	previousDataDir := flags.Global.DataDir

	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err = testDB.Exec(`
		CREATE TABLE users (
			id char(32) PRIMARY KEY,
			created_at datetime,
			updated_at datetime,
			username varchar(32) NOT NULL UNIQUE,
			email varchar(64) UNIQUE,
			hashed_password blob NOT NULL,
			role integer NOT NULL DEFAULT 2,
			registered_by_provider numeric NOT NULL DEFAULT false,
			registered_by_email numeric NOT NULL DEFAULT false
		)
	`).Error; err != nil {
		t.Fatalf("create users table: %v", err)
	}

	db = testDB
	dbType = conf.DatabaseTypeSqlite3
	conf.Conf = conf.DefaultConfig()
	flags.Global.DataDir = t.TempDir()

	t.Cleanup(func() {
		sqlDB, sqlErr := testDB.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
		db = previousDB
		dbType = previousDBType
		conf.Conf = previousConfig
		flags.Global.DataDir = previousDataDir
	})
}

func createTestRoot(t *testing.T, value string) {
	t.Helper()

	createTestRootNamed(t, "root", value)
}

func createTestRootNamed(t *testing.T, username, value string) *model.User {
	t.Helper()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash root password: %v", err)
	}
	root, err := CreateUserWithHashedPassword(
		username,
		hashedPassword,
		WithRole(model.RoleRoot),
	)
	if err != nil {
		t.Fatalf("create root user: %v", err)
	}

	return root
}

func getTestRoot(t *testing.T) *model.User {
	t.Helper()

	var root model.User
	if err := db.Where("role = ?", model.RoleRoot).First(&root).Error; err != nil {
		t.Fatalf("load root user: %v", err)
	}

	return &root
}

func getTestInitialRootPasswordFile(t *testing.T, user *model.User) string {
	t.Helper()

	path, err := initialRootPasswordFilePath(user.ID)
	if err != nil {
		t.Fatalf("initial root password file path: %v", err)
	}

	return path
}
