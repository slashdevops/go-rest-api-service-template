package app

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

// TestEveryConfigFieldIsReadFromTheEnvironment is the other half of
// TestEveryConfigFieldHasAFlag.
//
// A setting reaches the process two ways, and both are hand-written: the flag
// in setupFlags, and the environment variable in that group's ParseEnvVars.
// Missing the flag line stops the process loudly, which the sibling test
// catches. Missing the ParseEnvVars line does something worse — nothing at all.
// The variable is documented, an operator sets it, the service starts, and it
// silently keeps the default. Six settings had already drifted on the flag side
// before it was guarded; this is the side where the drift would not announce
// itself.
//
// It works by setting every declared variable to a value that cannot be a
// default and checking the setting actually moved, so it tests the wiring
// rather than restating it.
func TestEveryConfigFieldIsReadFromTheEnvironment(t *testing.T) {
	defaults := newConfigs()
	probed := newConfigs()

	var (
		skipped     []string
		probedCount int
	)

	// A directory for the settings that name a file. GetEnv opens the path with
	// O_CREATE, so it does not have to exist beforehand -- but it must not be a
	// path this test would mind creating.
	dir := t.TempDir()

	// Point every declared variable at a distinctive value.
	forEachField(reflect.ValueOf(probed), func(path string, f configField) {
		value, ok := probeValue(f.value, dir)
		if !ok {
			skipped = append(skipped, fmt.Sprintf("%s (%s)", f.envName, f.value.Type()))

			return
		}

		probedCount++

		t.Setenv(f.envName, value)
	})

	parseEnvVars(probed)

	var unread []string

	forEachField(reflect.ValueOf(probed), func(path string, f configField) {
		if _, ok := probeValue(f.value, dir); !ok {
			return
		}

		before := fieldByPath(reflect.ValueOf(defaults), path)
		if render(before) == render(f.value) {
			unread = append(unread, fmt.Sprintf("%s (%s)", f.envName, path))
		}
	})

	if len(unread) > 0 {
		t.Fatalf(
			"these settings did not change when their environment variable was set, so nothing reads them:\n  %s\n\n"+
				"add one line per setting to the group's ParseEnvVars in internal/config/",
			strings.Join(unread, "\n  "),
		)
	}

	// Say what was actually covered. A test that silently probed nothing would
	// pass just as loudly as one that probed everything.
	t.Logf("probed %d of %d declared settings", probedCount, probedCount+len(skipped))

	if len(skipped) > 0 {
		t.Errorf(
			"no probe value could be built for these settings, so nothing checks that they are read:\n  %s\n\n"+
				"teach probeValue about the type, rather than leaving the gap unstated",
			strings.Join(skipped, "\n  "),
		)
	}

	if probedCount == 0 {
		t.Fatal("no settings were probed; this test would pass by looking at nothing")
	}
}

type configField struct {
	value   reflect.Value
	envName string
}

// forEachField visits every config.Field under v, reporting the path to its
// Value and the environment variable that should feed it.
func forEachField(v reflect.Value, visit func(path string, f configField)) {
	walk(v, "", visit)
}

func walk(v reflect.Value, path string, visit func(string, configField)) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}

		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return
	}

	if env := v.FieldByName("EnVarName"); env.IsValid() && env.Kind() == reflect.String {
		if value := v.FieldByName("Value"); value.IsValid() && env.String() != "" {
			visit(path+".Value", configField{value: value, envName: env.String()})

			return
		}
	}

	for i := range v.NumField() {
		if v.Type().Field(i).IsExported() {
			walk(v.Field(i), path+"."+v.Type().Field(i).Name, visit)
		}
	}
}

func fieldByPath(v reflect.Value, path string) reflect.Value {
	for name := range strings.SplitSeq(strings.TrimPrefix(path, "."), ".") {
		for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
			v = v.Elem()
		}

		v = v.FieldByName(name)
	}

	return v
}

// render turns a setting into something comparable.
//
// Several settings are types with their own String() -- a file handle, a list --
// where comparing the raw value would compare an *os.File rather than the path
// it was opened from. Their String() is what the flag package and the operator
// both see, so it is what "did this change" should mean here.
func render(v reflect.Value) string {
	if v.CanAddr() {
		if s, ok := v.Addr().Interface().(fmt.Stringer); ok {
			return s.String()
		}
	}

	if s, ok := v.Interface().(fmt.Stringer); ok {
		return s.String()
	}

	return fmt.Sprint(v.Interface())
}

// probeValue returns a value that no default can already be, so that "the
// setting changed" is proof the environment was read.
//
// dir is where the file-valued settings are pointed; GetEnv opens the path with
// O_CREATE, so the file does not need to exist first.
func probeValue(v reflect.Value, dir string) (string, bool) {
	switch value := v.Interface().(type) {
	case config.FileVar:
		_ = value

		return filepath.Join(dir, "config-env-probe.file"), true
	case config.SliceStringVar:
		_ = value

		return "config-env-probe-entry", true
	}

	switch v.Kind() {
	case reflect.String:
		return "config-env-probe", true
	case reflect.Bool:
		// The only distinctive value for a bool is the opposite of its default.
		return fmt.Sprintf("%t", !v.Bool()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Type() == reflect.TypeFor[time.Duration]() {
			return "4321s", true
		}

		return "4321", true
	case reflect.Float32, reflect.Float64:
		return "4321.5", true
	default:
		return "", false
	}
}
