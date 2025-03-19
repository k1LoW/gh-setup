package setup

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSortPaths(t *testing.T) {
	t.Setenv("HOME", "/Users/k1low")

	tests := []struct {
		paths []string
		want  []string
	}{
		{
			[]string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/local/sbin", "/usr/bin", "/usr/sbin", "/Users/k1low/.local/bin"},
			[]string{"/Users/k1low/.local/bin", "/usr/local/bin", "/usr/bin"},
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s", tt.paths), func(t *testing.T) {
			got, err := sortPaths(tt.paths)
			if err != nil {
				t.Error(err)
			}
			if diff := cmp.Diff(got, tt.want, nil); diff != "" {
				t.Errorf("%s", diff)
			}
		})
	}
}

func TestChecksum(t *testing.T) {
	// Test data
	testData := []byte("hello world")

	tests := []struct {
		name        string
		data        []byte
		checksumStr string
		wantErr     bool
	}{
		// Empty checksum string (should return nil)
		{
			name:        "Empty checksum",
			data:        testData,
			checksumStr: "",
			wantErr:     false,
		},

		// Explicit algorithm specification (algorithm:hash format)
		// SHA-256 tests
		{
			name:        "SHA-256 explicit - correct",
			data:        testData,
			checksumStr: "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			wantErr:     false,
		},
		{
			name:        "SHA-256 explicit - incorrect",
			data:        testData,
			checksumStr: "sha256:incorrect_hash",
			wantErr:     true,
		},

		// SHA-512 tests
		{
			name:        "SHA-512 explicit - correct",
			data:        testData,
			checksumStr: "sha512:309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f",
			wantErr:     false,
		},
		{
			name:        "SHA-512 explicit - incorrect",
			data:        testData,
			checksumStr: "sha512:incorrect_hash",
			wantErr:     true,
		},

		// SHA-1 tests
		{
			name:        "SHA-1 explicit - correct",
			data:        testData,
			checksumStr: "sha1:2aae6c35c94fcfb415dbe95f408b9ce91ee846ed",
			wantErr:     false,
		},
		{
			name:        "SHA-1 explicit - incorrect",
			data:        testData,
			checksumStr: "sha1:incorrect_hash",
			wantErr:     true,
		},

		// MD5 tests
		{
			name:        "MD5 explicit - correct",
			data:        testData,
			checksumStr: "md5:5eb63bbbe01eeed093cb22bb8f5acdc3",
			wantErr:     false,
		},
		{
			name:        "MD5 explicit - incorrect",
			data:        testData,
			checksumStr: "md5:incorrect_hash",
			wantErr:     true,
		},

		// CRC32 tests
		{
			name:        "CRC32 explicit - correct",
			data:        testData,
			checksumStr: "crc32:0d4a1185",
			wantErr:     false,
		},
		{
			name:        "CRC32 explicit - incorrect",
			data:        testData,
			checksumStr: "crc32:incorrect",
			wantErr:     true,
		},

		// Invalid algorithm
		{
			name:        "Invalid algorithm",
			data:        testData,
			checksumStr: "invalid:hash",
			wantErr:     true,
		},

		// Automatic algorithm detection
		// SHA-256 auto-detection
		{
			name:        "SHA-256 auto-detection - correct",
			data:        testData,
			checksumStr: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			wantErr:     false,
		},
		{
			name:        "SHA-256 auto-detection - incorrect",
			data:        testData,
			checksumStr: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde8", // Changed last digit
			wantErr:     true,
		},

		// SHA-512 auto-detection
		{
			name:        "SHA-512 auto-detection - correct",
			data:        testData,
			checksumStr: "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f",
			wantErr:     false,
		},
		{
			name:        "SHA-512 auto-detection - incorrect",
			data:        testData,
			checksumStr: "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76e", // Changed last digit
			wantErr:     true,
		},

		// SHA-1 auto-detection
		{
			name:        "SHA-1 auto-detection - correct",
			data:        testData,
			checksumStr: "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed",
			wantErr:     false,
		},
		{
			name:        "SHA-1 auto-detection - incorrect",
			data:        testData,
			checksumStr: "2aae6c35c94fcfb415dbe95f408b9ce91ee846ec", // Changed last digit
			wantErr:     true,
		},

		// MD5 auto-detection
		{
			name:        "MD5 auto-detection - correct",
			data:        testData,
			checksumStr: "5eb63bbbe01eeed093cb22bb8f5acdc3",
			wantErr:     false,
		},
		{
			name:        "MD5 auto-detection - incorrect",
			data:        testData,
			checksumStr: "5eb63bbbe01eeed093cb22bb8f5acdc2", // Changed last digit
			wantErr:     true,
		},

		// CRC32 auto-detection
		{
			name:        "CRC32 auto-detection - correct",
			data:        testData,
			checksumStr: "0d4a1185",
			wantErr:     false,
		},
		{
			name:        "CRC32 auto-detection - incorrect",
			data:        testData,
			checksumStr: "0d4a1184", // Changed last digit
			wantErr:     true,
		},

		// Invalid length for auto-detection
		{
			name:        "Invalid length for auto-detection",
			data:        testData,
			checksumStr: "invalid_length_hash",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checksum(tt.data, tt.checksumStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("checksum() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
