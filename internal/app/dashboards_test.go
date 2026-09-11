//go:build unit

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Grafana dashboards are provisioned from dev-env/ and nothing validates
// them before Grafana loads them.
//
// # Why a test and not a linter
//
// grafana/dashboard-linter is the obvious tool and neither of its two
// distributions can be used here: it publishes no container image, and
// `go install` refuses it because its go.mod carries replace directives. A
// Makefile target that cannot run is worse than none.
//
// A test is a better home anyway. `make check-alerts` has to be remembered;
// this runs in `make test` and in CI, which is where a broken dashboard should
// be caught -- the alternative is finding out from an empty panel, and an empty
// panel looks exactly like a service that is not being used.
//
// # What it checks
//
// Only failures that are silent in Grafana. A panel pointing at a datasource
// uid that does not exist renders "Datasource not found" in one panel while
// every other panel on the dashboard works, which is easy to miss on a
// dashboard with twenty of them.

// dashboardDir is where the provisioned dashboards live.
func dashboardDir(t *testing.T) string {
	t.Helper()

	return filepath.Join(repoRootFrom(t), "dev-env", "configuration", "grafana", "dashboard")
}

// datasourceUIDs reads the uids the provisioning file actually defines.
func datasourceUIDs(t *testing.T) map[string]bool {
	t.Helper()

	path := filepath.Join(repoRootFrom(t), "dev-env", "configuration", "grafana", "datasource", "grafana-ds.yaml")

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	uids := map[string]bool{}

	for line := range strings.SplitSeq(string(body), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "uid:"); ok {
			uids[strings.TrimSpace(after)] = true
		}
	}

	if len(uids) == 0 {
		t.Fatal("parsed no datasource uids out of grafana-ds.yaml; the parser has lost track of the format")
	}

	return uids
}

type dashboardPanel struct {
	Datasource *struct {
		UID string `json:"uid"`
	} `json:"datasource"`
	Type    string           `json:"type"`
	Title   string           `json:"title"`
	Targets []dashboardPanel `json:"targets"`
	Panels  []dashboardPanel `json:"panels"`
	ID      int              `json:"id"`
}

type dashboard struct {
	UID    string           `json:"uid"`
	Title  string           `json:"title"`
	Panels []dashboardPanel `json:"panels"`
}

func dashboardFiles(t *testing.T) []string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dashboardDir(t), "*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("no dashboards found; this test is looking in the wrong place")
	}

	return files
}

// flatten yields a panel and every panel nested inside a collapsed row.
func flatten(panels []dashboardPanel) []dashboardPanel {
	out := make([]dashboardPanel, 0, len(panels))

	for _, p := range panels {
		out = append(out, p)
		out = append(out, flatten(p.Panels)...)
	}

	return out
}

// A panel whose datasource does not exist renders "Datasource not found" in
// that panel alone, while the rest of the dashboard works.
func TestDashboardsOnlyReferenceDeclaredDatasources(t *testing.T) {
	t.Parallel()

	known := datasourceUIDs(t)

	for _, file := range dashboardFiles(t) {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".json"), func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			var d dashboard
			if err := json.Unmarshal(body, &d); err != nil {
				t.Fatalf("a provisioned dashboard must be valid JSON: %v", err)
			}

			for _, p := range flatten(d.Panels) {
				for _, ds := range append([]dashboardPanel{p}, p.Targets...) {
					if ds.Datasource == nil {
						continue
					}

					uid := ds.Datasource.UID

					// A dashboard variable as the datasource is resolved at
					// render time and cannot be checked from here.
					if uid == "" || strings.HasPrefix(uid, "$") {
						continue
					}

					if !known[uid] {
						t.Errorf("panel %q (id %d) points at datasource uid %q, which grafana-ds.yaml does not define", p.Title, p.ID, uid)
					}
				}
			}
		})
	}
}

// Two panels sharing an id makes Grafana drop one of them on save, and the
// loss is silent.
func TestDashboardPanelIDsAreUnique(t *testing.T) {
	t.Parallel()

	for _, file := range dashboardFiles(t) {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".json"), func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			var d dashboard
			if err := json.Unmarshal(body, &d); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			seen := map[int]string{}

			for _, p := range flatten(d.Panels) {
				if p.ID == 0 {
					continue
				}

				if first, clash := seen[p.ID]; clash {
					t.Errorf("panel id %d is used by both %q and %q", p.ID, first, p.Title)
				}

				seen[p.ID] = p.Title
			}
		})
	}
}

// Every dashboard needs a stable uid: provisioning keys on it, so a dashboard
// that loses or changes one is re-created as a duplicate rather than updated,
// and every link and bookmark to the old one breaks.
func TestDashboardsHaveAStableUID(t *testing.T) {
	t.Parallel()

	seen := map[string]string{}

	for _, file := range dashboardFiles(t) {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		var d dashboard
		if err := json.Unmarshal(body, &d); err != nil {
			t.Fatalf("%s: invalid JSON: %v", filepath.Base(file), err)
		}

		if d.UID == "" {
			t.Errorf("%s has no uid", filepath.Base(file))

			continue
		}

		if first, clash := seen[d.UID]; clash {
			t.Errorf("uid %q is used by both %s and %s", d.UID, first, filepath.Base(file))
		}

		seen[d.UID] = filepath.Base(file)
	}
}

// The logs panels are the reason Loki is in the pod. A dashboard that quietly
// loses them leaves the third signal invisible, which is the state this whole
// change exists to end.
func TestObservabilityDashboardQueriesLoki(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(dashboardDir(t), "go-rest-api-service-template-observability.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var d dashboard
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, p := range flatten(d.Panels) {
		if p.Datasource != nil && p.Datasource.UID == "loki" {
			return
		}
	}

	t.Error("the observability dashboard has no panel querying Loki; logs are the third signal and this is where they are shown")
}
