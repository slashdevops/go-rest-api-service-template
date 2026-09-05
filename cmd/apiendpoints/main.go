package main

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"uuid"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/version"
)

var (
	appName = "apiendpoints"

	showVersion     bool
	showLongVersion bool
	showHelp        bool
	swaggerJSONFile = config.Field[string]{Value: "./docs/api/swagger.json"}
)

func init() {
	// Version, Help and debug flags
	flag.BoolVar(&showVersion, "version", false, "Show the version information")
	flag.BoolVar(&showLongVersion, "version.long", false, "Show the long version information")
	flag.BoolVar(&showHelp, "help", false, "Show this help message")

	// Swagger JSON file
	flag.StringVar(&swaggerJSONFile.Value, "swagger.file", swaggerJSONFile.Value, "Path to the swagger.json file")

	// Parse the command line arguments
	flag.Parse()

	flag.Usage = func() {
		_, err := fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\nOptions:\n", appName)
		if err != nil {
			slog.Error("failed to print usage", "error", err)
			os.Exit(1)
		}

		flag.PrintDefaults()
	}

	// implement the version flag
	if showVersion {
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

	// implement the long version flag
	if showLongVersion {
		var sb strings.Builder

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

		fmt.Print(sb.String())

		os.Exit(0)
	}

	// implement the help flag
	if showHelp {
		flag.Usage()
		os.Exit(0)
	}
}

func main() {
	// Open the swagger.json file
	file, err := os.Open(swaggerJSONFile.Value)
	if err != nil {
		slog.Error("failed to open swagger.json file", "error", err)
		os.Exit(1)
	}
	defer file.Close()

	// decode the swagger.json file
	var sw swagger
	if err := json.NewDecoder(file).Decode(&sw); err != nil {
		slog.Error("failed to decode swagger.json file", "error", err)
		os.Exit(1)
	}

	excludes := map[string]string{
		"/ui":                 "GET",
		"/projects/health":    "GET",
		"/permissions/health": "GET",
		"/roles/health":       "GET",
		"/tokens/health":      "GET",
		"/users/health":       "GET",
		"/version":            "GET",
		"/health/status":      "GET",
	}

	var records []Record
	idMaxWidth := len("ID")
	summaryMaxWidth := len("Summary")
	descriptionMaxWidth := len("Description")
	methodMaxWidth := len("Method")
	pathMaxWidth := len("Path")
	systemMaxWidth := len("System")

	for path, methods := range sw.Paths {
		for method, data := range methods {
			// skip the excludes paths
			if val, ok := excludes[path]; ok {
				if val == strings.ToUpper(method) {
					continue
				}
			}

			var id uuid.UUID
			if data.OperationID == "" {
				id = uuid.NewV7()
			} else {
				id, err = uuid.Parse(data.OperationID)
				if err != nil {
					panic(err)
				}
			}

			if len(id.String()) > idMaxWidth {
				idMaxWidth = len(id.String())
			}

			if len(data.Summary) > summaryMaxWidth {
				summaryMaxWidth = len(data.Summary)
			}

			if len(data.Description) > descriptionMaxWidth {
				descriptionMaxWidth = len(data.Description)
			}

			if len(method) > methodMaxWidth {
				methodMaxWidth = len(method)
			}

			if len(path) > pathMaxWidth {
				pathMaxWidth = len(path)
			}

			if len("TRUE") > systemMaxWidth {
				systemMaxWidth = len("TRUE")
			}

			record := Record{
				ID:          sqlQuote(id.String()),
				Summary:     sqlQuote(data.Summary),
				Description: sqlQuote(data.Description),
				Method:      sqlQuote(strings.ToUpper(method)),
				Path:        sqlQuote(path),
				System:      "TRUE",
			}

			records = append(records, record)
		}
	}

	// sort the records by path
	// Sort on (Path, Method), not Path alone.
	//
	// The output of this program is pasted into
	// database/migrations/3100_roles_policies_tables_upsert.sql, so the ordering
	// has to be a function of the swagger spec and nothing else. Two things
	// conspired against that:
	//
	//   - sw.Paths is a map, and Go randomises map iteration order, so the slice
	//     reaches the sort in a different order on every run;
	//   - sorting on Path alone leaves every same-path/different-method group
	//     tied, and sort.Slice is not stable, so tied rows came out in whatever
	//     order the randomised walk happened to produce.
	//
	// The rows were always the same 136; only their order moved. That is worse
	// than it sounds — regenerating produced a large diff every single time, so
	// a real change to the endpoint set was indistinguishable from noise.
	//
	// (Path, Method) is unique per operation, so this comparator is total and
	// there are no ties left for stability to matter.
	slices.SortFunc(records, func(a, b Record) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Method, b.Method),
		)
	})

	idMaxWidth += 2
	summaryMaxWidth += 2
	descriptionMaxWidth += 2 // Add padding for description
	methodMaxWidth += 2
	pathMaxWidth += 2
	systemMaxWidth += 2 // Consistent padding

	for i, record := range records {
		if i == len(records)-1 {
			_, err := fmt.Printf(
				"(%-*s, %-*s, %-*s, %-*s, %-*s, %-*s);\n",
				idMaxWidth, record.ID,
				summaryMaxWidth, record.Summary,
				descriptionMaxWidth, record.Description,
				methodMaxWidth, record.Method,
				pathMaxWidth, record.Path,
				systemMaxWidth, record.System,
			)
			if err != nil {
				slog.Error("failed to print record", "error", err)
				os.Exit(1)
			}
		} else {
			_, err := fmt.Printf(
				"(%-*s, %-*s, %-*s, %-*s, %-*s, %-*s),\n",
				idMaxWidth, record.ID,
				summaryMaxWidth, record.Summary,
				descriptionMaxWidth, record.Description,
				methodMaxWidth, record.Method,
				pathMaxWidth, record.Path,
				systemMaxWidth, record.System,
			)
			if err != nil {
				slog.Error("failed to print record", "error", err)
				os.Exit(1)
			}
		}
	}
}

// sqlQuote wraps a value in single quotes and escapes any it contains.
//
// Without this, one apostrophe in a handler's @Summary or @Description -- "a
// product's name" -- ends the SQL string literal early and the generated
// migration does not parse. The generator emitted the value raw, so the failure
// showed up as a syntax error in a 90-line INSERT rather than anywhere near the
// annotation that caused it.
func sqlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

type Record struct {
	ID          string
	Summary     string
	Description string
	Method      string
	Path        string
	System      string
}

// swagger is the struct that represents the swagger.json file
type swagger struct {
	Paths map[string]map[string]struct {
		Responses map[string]struct {
			Description string `json:"description"`
			Schema      struct {
				Type string `json:"type"`
			} `json:"schema"`
		} `json:"responses"`
		Description string   `json:"description"`
		Summary     string   `json:"summary"`
		OperationID string   `json:"operationId"`
		Consumes    []string `json:"consumes"`
		Produces    []string `json:"produces"`
		Tags        []string `json:"tags"`
	} `json:"paths"`
	Info struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Version     string `json:"version"`
	} `json:"info"`
	Swagger string `json:"swagger"`
}
