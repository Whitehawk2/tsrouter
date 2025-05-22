package main

import (
	"flag"
	"os"
	"reflect"
	"testing"

	"github.com/whitehawk2/tsrouter/models"
)

func TestObscureCredential(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "***"},
		{"short string", "123", "***"},
		{"medium string", "12345678", "***"}, // Based on current logic <=8 is "***"
		{"long string", "123456789", "1234...6789"},
		{"exact boundary + 1", "abcdefghi", "abcd...fghi"}, // 9 chars
		{"string of 8 chars", "abcdefgh", "***"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := obscureCredential(tc.input)
			if got != tc.expected {
				t.Errorf("obscureCredential(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	// Helper function to reset flags and os.Args for each subtest
	resetFlags := func() {
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		// flag.Usage is also reset by NewFlagSet
	}

	// Store original os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	t.Run("all flags provided", func(t *testing.T) {
		resetFlags()
		os.Args = []string{
			"cmd",
			"--target-port=8080",
			"--hostname=test-host",
			"--log-level=debug",
			"--auth-key-expiry-days=30",
		}

		expected := &models.Config{
			TargetPort:        8080,
			Hostname:          "test-host",
			LogLevel:          "debug",
			AuthKeyExpiryDays: 30,
		}

		cfg := parseFlags()
		if !reflect.DeepEqual(cfg, expected) {
			t.Errorf("parseFlags() got = %+v, want %+v", cfg, expected)
		}
	})

	t.Run("default values for optional flags", func(t *testing.T) {
		resetFlags()
		os.Args = []string{
			"cmd",
			"--target-port=9000",
			"--hostname=default-host",
		}

		expected := &models.Config{
			TargetPort:        9000,
			Hostname:          "default-host",
			LogLevel:          "info", // default
			AuthKeyExpiryDays: 14,   // default
		}

		cfg := parseFlags()
		if !reflect.DeepEqual(cfg, expected) {
			t.Errorf("parseFlags() got = %+v, want %+v", cfg, expected)
		}
	})

	t.Run("missing required flags", func(t *testing.T) {
		// Note: This test case cannot fully verify os.Exit(1) behavior
		// without more complex setup (e.g., execing a subprocess or mocking os.Exit).
		// The current parseFlags() function calls os.Exit(1) directly if required flags are missing,
		// which terminates the test process.
		// We are testing that parseFlags *would* try to exit by checking if flag.Usage is called.
		// For this, we need a way to capture that. A simple way is to temporarily replace flag.Usage.

		resetFlags()
		// origUsage := flag.Usage // Keep for potential future use if os.Exit can be mocked
		// flag.Usage = func() {
		// 	 // Potentially set a flag here or panic for test purposes
		//	 // origUsage() // Call original usage if desired, or skip to prevent exit
		// }
		// defer func() { flag.Usage = origUsage }()

		// Test with missing --hostname
		os.Args = []string{"cmd", "--target-port=1234"}

		// The call to parseFlags() below would normally call os.Exit(1), terminating this test run.
		// There isn't a clean way to catch this with the current structure of parseFlags
		// and the limitations of the testing environment for mocking os.Exit.
		// We acknowledge this limitation.
		// If parseFlags were refactored to return an error, we could test for that error here.

		// cfg := parseFlags() // This line would cause the test to exit.

		t.Log("Skipping direct assertion for missing required flags due to os.Exit(1) in parseFlags. Manual verification, refactoring parseFlags to return an error, or more complex test setup (e.g., exec) would be needed for this path.")
	})
}
