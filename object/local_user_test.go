package object

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func useLocalUserTestDatabase(t *testing.T) {
	t.Helper()
	adapter := NewAdapter("sqlite", filepath.Join(t.TempDir(), "casos.db"), "")
	adapter.createTable()
	previous := ormer
	ormer = adapter
	t.Cleanup(func() {
		ormer = previous
		adapter.close()
	})
}

func TestLocalAdminLifecycle(t *testing.T) {
	useLocalUserTestDatabase(t)

	initialized, err := IsLocalAdminInitialized()
	if err != nil {
		t.Fatalf("check initial state: %v", err)
	}
	if initialized {
		t.Fatal("fresh database should require local setup")
	}

	user, err := CreateLocalAdmin("correct horse battery staple")
	if err != nil {
		t.Fatalf("create local administrator: %v", err)
	}
	if user.Name != LocalAdminName || user.Owner != LocalUserOwner || !user.IsAdmin {
		t.Fatalf("unexpected local administrator: %#v", user)
	}
	if user.PasswordHash == "correct horse battery staple" || user.PasswordHash == "" {
		t.Fatal("password was not hashed")
	}

	if _, err = CreateLocalAdmin("another secure password"); !errors.Is(err, ErrLocalAdminAlreadyInitialized) {
		t.Fatalf("second setup error = %v, want ErrLocalAdminAlreadyInitialized", err)
	}

	verified, ok, err := VerifyLocalUser(LocalAdminName, "correct horse battery staple")
	if err != nil || !ok || verified == nil {
		t.Fatalf("verify administrator = (%#v, %t, %v), want success", verified, ok, err)
	}
	if _, ok, err = VerifyLocalUser(LocalAdminName, "wrong password"); err != nil || ok {
		t.Fatalf("wrong password verified = %t, err = %v", ok, err)
	}
	if _, ok, err = VerifyLocalUser("unknown", "wrong password"); err != nil || ok {
		t.Fatalf("unknown user verified = %t, err = %v", ok, err)
	}

	previousUpdatedTime := user.UpdatedTime
	if err = UpdateLocalUserPassword(user, "a different secure password"); err != nil {
		t.Fatalf("update password: %v", err)
	}
	if user.UpdatedTime == previousUpdatedTime {
		t.Fatal("password update did not advance UpdatedTime")
	}
	if _, ok, _ = VerifyLocalUser(LocalAdminName, "correct horse battery staple"); ok {
		t.Fatal("old password still works")
	}
	if _, ok, err = VerifyLocalUser(LocalAdminName, "a different secure password"); err != nil || !ok {
		t.Fatalf("new password verified = %t, err = %v", ok, err)
	}
}

func TestValidateLocalPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "empty", password: "", wantErr: true},
		{name: "whitespace", password: "        ", wantErr: true},
		{name: "short", password: "1234567", wantErr: true},
		{name: "minimum", password: "12345678"},
		{name: "unicode characters", password: "密码安全测试一二"},
		{name: "bcrypt byte limit", password: strings.Repeat("a", maxLocalPasswordBytes+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLocalPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateLocalPassword() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
