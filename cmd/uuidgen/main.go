package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/version"
)

const (
	appName = "uuidgen"
)

func main() {
	showVersion := flag.Bool("version", false, "Show version")
	showVersionLong := flag.Bool("version.long", false, "Show long version")
	showHelp := flag.Bool("help", false, "Show help")

	num := flag.Int("n", 10, "number of UUIDs to generate")
	ver := flag.Int("v", 7, "version of UUID to generate. Supported versions: 4, 7")
	flag.Parse()

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

	// check the flags values
	if *num < 1 || *num > 1000 {
		flag.Usage()
		fmt.Printf("Number of UUIDs to generate must be between 1 and 1000, got: %d\n", *num)
		return
	}

	// v6 is gone: the standard library uuid package has no NewV6 and nothing in
	// this service ever generated one. Every ID the service mints is v7.
	if *ver != 4 && *ver != 7 {
		flag.Usage()
		fmt.Printf("Unsupported UUID version: %d. Supported versions are 4 and 7.\n", *ver)
		return
	}

	for range *num {
		var u uuid.UUID

		switch *ver {
		case 7:
			u = uuid.NewV7()
		case 4:
			u = uuid.NewV4()
		default:
			fmt.Printf("Unsupported UUID version: %d\n", *ver)
			return
		}

		// Print the generated UUID
		if _, err := fmt.Println(u.String()); err != nil {
			panic(err)
		}
	}
}
