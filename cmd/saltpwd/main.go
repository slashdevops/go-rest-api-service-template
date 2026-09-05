package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/slashdevops/go-rest-api-service-template/internal/version"
)

const (
	validMinPasswordLength     = 3
	validMaxPasswordLength     = 20
	validMinHashPasswordLength = 10
	validMaxHashPasswordLength = 200

	appName = "saltpwd"
)

// HashAndSaltPassword hashes and salts the password
// It uses bcrypt to hash the password with a cost of 10.
// The hashed password is returned as a string.
func HashAndSaltPassword(password string, cost ...int) (string, error) {
	var costVal int
	if len(cost) > 0 {
		if cost[0] < bcrypt.MinCost || cost[0] > bcrypt.MaxCost {
			return "", fmt.Errorf("cost value must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
		}
		costVal = cost[0]
	} else {
		costVal = bcrypt.DefaultCost
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), costVal)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

// ComparePasswords compares the hashed password and the plain password
// It uses bcrypt to compare the hashed password with the plain password.
// It returns true if the passwords match, false otherwise.
func ComparePasswords(hashedPwd string, plainPwd string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPwd), []byte(plainPwd))
	return err == nil
}

func main() {
	showVersion := flag.Bool("version", false, "Show version")
	showVersionLong := flag.Bool("version.long", false, "Show long version")
	showHelp := flag.Bool("help", false, "Show help")

	plainPwd := flag.String("password", "", "Password to hash. Must be quoted in single quotes ('')")
	cost := flag.Int("cost", bcrypt.DefaultCost, "Cost for bcrypt hashing")
	hashedPwd := flag.String("hashed", "", "[OPTIONAL] Hashed password to compare with.  In case of empty string, it will not be used, must be quoted in single quotes ('')")
	flag.Parse()

	flag.Usage = func() {
		output := flag.CommandLine.Output()
		_, err := fmt.Fprintf(output, "Usage: %s [options]\n\n", appName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error printing usage header: %v\n", err)
			os.Exit(1)
		}
		_, err = fmt.Fprintf(output, "Utility to hash passwords using bcrypt or compare a plain password against a bcrypt hash.\n\nOptions:\n")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error printing usage description: %v\n", err)
			os.Exit(1)
		}

		flag.PrintDefaults()

		_, err = fmt.Fprintf(output, "\nExample (Generate Hash):\n  %s -password='yourSecretPassword'\n", appName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error printing usage example 1: %v\n", err)
			os.Exit(1)
		}

		_, err = fmt.Fprintf(output, "\n  %s -password 'yourSecretPassword'\n", appName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error printing usage example 1: %v\n", err)
			os.Exit(1)
		}

		_, err = fmt.Fprintf(output, "\nExample (Compare Hash):\n  %s -password='yourSecretPassword' -hashed='$2a$10$...' \n", appName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error printing usage example 2: %v\n", err)
			os.Exit(1)
		}

		_, err = fmt.Fprintf(output, "\n  %s -password 'yourSecretPassword' -hashed '$2a$10$...'\n", appName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error printing usage example 2: %v\n", err)
			os.Exit(1)
		}

		_, err = fmt.Fprintf(output, "\nImportant: Always enclose password and hashed values in single quotes ('') \n"+
			"           to prevent shell interpretation of special characters.\n")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error printing usage note: %v\n", err)
			os.Exit(1)
		}
	}

	if *showVersion {
		if version.Version == "0.0.0" {
			if info, ok := debug.ReadBuildInfo(); ok {
				fmt.Printf("Version: %s\n", info.Main.Version)
			} else {
				fmt.Printf("Version: %s\n", version.Version)
			}
		} else {
			fmt.Printf("Version: %s\n", version.Version)
		}

		os.Exit(0)
	}

	if *showVersionLong {
		var sb strings.Builder

		if version.Version == "0.0.0" {
			if info, ok := debug.ReadBuildInfo(); ok {
				fmt.Fprintf(&sb, "%s version: %s, ", appName, info.Main.Version)
				fmt.Fprintf(&sb, "Git commit: %s, ", info.Main.Sum)
				fmt.Fprintf(&sb, "Go version: %s\n", info.GoVersion)
			} else {
				fmt.Fprintf(&sb, "%s version: %s, ", appName, version.Version)
				fmt.Fprintf(&sb, "Build date: %s, ", version.BuildDate)
				fmt.Fprintf(&sb, "Build user: %s, ", version.BuildUser)
				fmt.Fprintf(&sb, "Git commit: %s, ", version.GitCommit)
				fmt.Fprintf(&sb, "Git branch: %s, ", version.GitBranch)
				fmt.Fprintf(&sb, "Go version: %s\n", version.GoVersion)
			}
		} else {
			fmt.Fprintf(&sb, "%s version: %s, ", appName, version.Version)
			fmt.Fprintf(&sb, "Build date: %s, ", version.BuildDate)
			fmt.Fprintf(&sb, "Build user: %s, ", version.BuildUser)
			fmt.Fprintf(&sb, "Git commit: %s, ", version.GitCommit)
			fmt.Fprintf(&sb, "Git branch: %s, ", version.GitBranch)
			fmt.Fprintf(&sb, "Go version: %s\n", version.GoVersion)
		}

		fmt.Print(sb.String())

		os.Exit(0)
	}

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	// --- Input Validation ---
	if plainPwd == nil || *plainPwd == "" {
		flag.Usage()
		fmt.Println()
		fmt.Fprintf(os.Stderr, "Error: Password is required\n")
		os.Exit(1)
	}

	if len(*plainPwd) < validMinPasswordLength || len(*plainPwd) > validMaxPasswordLength {
		fmt.Fprintf(os.Stderr, "Error: Password length must be between %d and %d characters. Provided length: %d\n", validMinPasswordLength, validMaxPasswordLength, len(*plainPwd))
		os.Exit(1)
	}

	// Validate cost only if generating hash (hashedPwd is empty)
	if (hashedPwd == nil || *hashedPwd == "") && (*cost < bcrypt.MinCost || *cost > bcrypt.MaxCost) {
		fmt.Fprintf(os.Stderr, "Error: Cost value must be between %d and %d. Provided value: %d\n", bcrypt.MinCost, bcrypt.MaxCost, *cost)
		os.Exit(1)
	}

	// Validate hashed password length only if provided
	if hashedPwd != nil && *hashedPwd != "" {
		if len(*hashedPwd) < validMinHashPasswordLength || len(*hashedPwd) > validMaxHashPasswordLength {
			fmt.Fprintf(os.Stderr, "Error: Hashed password length must be between %d and %d characters. Provided length: %d\n", validMinHashPasswordLength, validMaxHashPasswordLength, len(*hashedPwd))
			os.Exit(1)
		}
	}

	// --- Main Logic: Compare or Hash ---
	if hashedPwd != nil && *hashedPwd != "" {
		// Mode: Compare passwords
		fmt.Printf("Comparing provided password against hash: %s\n", *hashedPwd)

		if ComparePasswords(*hashedPwd, *plainPwd) {
			fmt.Println("Result: Passwords match!")
		} else {
			fmt.Println("Result: Passwords do not match.")
		}
	} else {
		// Mode: Generate hash
		fmt.Printf("Generating hash for the provided password with cost %d...\n", *cost)
		hashed, err := HashAndSaltPassword(*plainPwd, *cost)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error hashing password: %v\n", err)
			os.Exit(1)
		} else {
			fmt.Printf("Generated Hashed password: %s\n", hashed)
		}
	}
}
