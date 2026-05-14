package repository

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateDSN_EmptyDSN(t *testing.T) {
	err := validateTestDSN("", "iris")
	if !errors.Is(err, errEmptyDSN) {
		t.Errorf("expected errEmptyDSN, got %v", err)
	}
}

func TestValidateDSN_SafelistIrisTest(t *testing.T) {
	dsn := "postgres://user:pass@localhost/iris_test"
	err := validateTestDSN(dsn, "iris")
	if err != nil {
		t.Errorf("expected nil for iris_test, got %v", err)
	}
}

func TestValidateDSN_SafelistIrisRepoTest(t *testing.T) {
	dsn := "postgres://user:pass@localhost/iris_repo_test"
	err := validateTestDSN(dsn, "iris")
	if err != nil {
		t.Errorf("expected nil for iris_repo_test, got %v", err)
	}
}

func TestValidateDSN_LiveDatabaseRejected(t *testing.T) {
	dsn := "postgres://user:pass@localhost/iris"
	err := validateTestDSN(dsn, "iris")
	if err == nil {
		t.Error("expected error for live database, got nil")
	}
	if !strings.Contains(err.Error(), "live database") {
		t.Errorf("expected 'live database' in error, got %v", err)
	}
}

func TestValidateDSN_LiveDatabaseDifferentLiveDB(t *testing.T) {
	dsn := "postgres://user:pass@localhost/iris"
	err := validateTestDSN(dsn, "iris_other")
	if err == nil {
		t.Error("expected error when dbname has no 'test' substring, got nil")
	}
}

func TestValidateDSN_TestSubstringAccepted(t *testing.T) {
	dsn := "postgres://user:pass@localhost/something_test"
	err := validateTestDSN(dsn, "iris")
	if err != nil {
		t.Errorf("expected nil for something_test, got %v", err)
	}
}

func TestValidateDSN_TestSubstringAccepted2(t *testing.T) {
	dsn := "postgres://user:pass@localhost/test_db"
	err := validateTestDSN(dsn, "iris")
	if err != nil {
		t.Errorf("expected nil for test_db, got %v", err)
	}
}

func TestValidateDSN_GarbledDSN(t *testing.T) {
	dsn := "not a valid url at all"
	err := validateTestDSN(dsn, "iris")
	if err == nil {
		t.Error("expected error for garbled DSN, got nil")
	}
}

func TestValidateDSN_NoDatabaseName(t *testing.T) {
	dsn := "postgres://user:pass@localhost"
	err := validateTestDSN(dsn, "iris")
	if err == nil {
		t.Error("expected error for DSN with no database name, got nil")
	}
}
