package app

import (
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestEveryConfigFieldHasAFlag walks the configuration by reflection and
// requires that every declared setting is reachable from the command line.
//
// # Why this exists
//
// A setting is declared in internal/config as a Field, which carries both its
// flag name and its environment variable name — but the flag itself is
// registered by hand, in setupFlags, one line per setting. Adding a Field and
// forgetting the line yields a setting that works through the environment and
// **fails the process** when passed as a flag:
//
//	flag provided but not defined: -authn.refresh.token.rotation.enabled
//
// That is worse than a missing feature. It looks like the setting does not
// exist, and it takes down a deployment that tries to use it. Six settings had
// drifted this way before this test existed, including the switch that turns
// refresh-token rotation off — the one thing an operator would reach for in a
// hurry, documented as available, and guaranteed to refuse to start.
//
// The environment-variable half drifts more quietly still — a missing
// ParseEnvVars line means the variable is simply ignored — and is covered by
// TestEveryConfigFieldIsReadFromTheEnvironment.
func TestEveryConfigFieldHasAFlag(t *testing.T) {
	// setupFlags registers on the global flag.CommandLine, so calling it twice
	// in one process panics with "flag redefined". Swap in a fresh set for the
	// duration: without this the package cannot be run with -count=2, and a
	// test that cannot be re-run is a test that cannot be used to chase a
	// flake.
	restore := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	t.Cleanup(func() { flag.CommandLine = restore })

	configs := newConfigs()
	setupFlags(configs)

	var missing []string

	walkConfigFields(reflect.ValueOf(configs), func(flagName string) {
		if flagName == "" {
			return
		}

		if flag.Lookup(flagName) == nil {
			missing = append(missing, flagName)
		}
	})

	if len(missing) > 0 {
		t.Fatalf(
			"these settings are declared but not registered as flags, so passing them stops the process:\n  %s\n\n"+
				"add one line per setting to setupFlags in internal/app/configs.go",
			strings.Join(missing, "\n  "),
		)
	}
}

// walkConfigFields visits every config.Field found under v, at any depth, and
// reports its FlagName. A Field is recognised structurally — a struct with
// FlagName, FlagDescription and EnVarName strings — so a new config group is
// covered the moment it is added, without this test being told about it.
func walkConfigFields(v reflect.Value, visit func(flagName string)) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}

		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	if name := fieldFlagName(v); name != "" {
		visit(name)

		return
	}

	for i := range v.NumField() {
		if v.Type().Field(i).IsExported() {
			walkConfigFields(v.Field(i), visit)
		}
	}
}

// fieldFlagName returns the FlagName when v is a config.Field, and "" otherwise.
func fieldFlagName(v reflect.Value) string {
	flagName := v.FieldByName("FlagName")
	if !flagName.IsValid() || flagName.Kind() != reflect.String {
		return ""
	}

	for _, sibling := range []string{"FlagDescription", "EnVarName"} {
		f := v.FieldByName(sibling)
		if !f.IsValid() || f.Kind() != reflect.String {
			return ""
		}
	}

	return flagName.String()
}
