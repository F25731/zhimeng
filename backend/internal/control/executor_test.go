package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/F25731/zhimeng/backend/internal/config"
)

type recordingRunner struct {
	calls   []recordedCall
	handler func(string, []string) (string, error)
}

type recordedCall struct {
	name string
	args []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.calls = append(r.calls, recordedCall{name: name, args: append([]string(nil), args...)})
	if r.handler != nil {
		return r.handler(name, args)
	}
	return "", nil
}

func TestDatabaseURLUsesPerSiteCredentials(t *testing.T) {
	value, err := databaseURL("postgres://control:admin@control-postgres:5432/postgres?sslmode=disable", "user_site", "p@ss:/word", "site_db")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.User.Username() != "user_site" || password != "p@ss:/word" || parsed.Path != "/site_db" {
		t.Fatalf("unexpected database URL: %s", value)
	}
}

func TestSiteTemplatesRenderAllRuntimeFields(t *testing.T) {
	data := siteTemplateData{
		SiteID: "site-id", Domain: "demo.juheai.club", SiteImage: "example/site:v1",
		ReporterImage: "example/reporter:v1", DockerNetwork: "platform-proxy",
		DatabaseNetwork: "control-private", DatabaseURL: "postgres://user:pass@db/site",
		EncryptionKey: "encryption", MaintenanceToken: "maintenance",
		ControlCenterURL: "https://open.juheai.club", ControlSiteToken: "report-token", Version: "v1",
		RouteEnabled: true,
	}
	for _, name := range []string{"site-env.tmpl", "site-compose.yml.tmpl"} {
		path := filepath.Join("..", "..", "..", "deploy", "templates", name)
		tpl, err := template.ParseFiles(path)
		if err != nil {
			t.Fatal(err)
		}
		var rendered bytes.Buffer
		if err := tpl.Execute(&rendered, data); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(rendered.String(), "<no value>") || !strings.Contains(rendered.String(), "site-id") {
			t.Fatalf("template %s did not render correctly", name)
		}
		if name == "site-compose.yml.tmpl" {
			if !strings.Contains(rendered.String(), "traefik.enable=true") ||
				!strings.Contains(rendered.String(), "Host(`demo.juheai.club`)") ||
				!strings.Contains(rendered.String(), "X-Control-Site-ID=site-id") ||
				!strings.Contains(rendered.String(), "VOZEB_PRO_WORKER_API_ORIGIN: ${VOZEB_PRO_WORKER_API_ORIGIN}") ||
				!strings.Contains(rendered.String(), "DATABASE_URL: ${SITE_DATABASE_URL}") ||
				strings.Contains(rendered.String(), "DATABASE_URL: ${DATABASE_URL}") {
				t.Fatalf("template %s is missing guarded host routing labels", name)
			}
			workerBlock := strings.Split(strings.Split(rendered.String(), "  worker:")[1], "  reporter:")[0]
			if strings.Contains(workerBlock, "DATABASE_URL") || strings.Contains(workerBlock, "env_file") {
				t.Fatal("worker service received database credentials")
			}
		}
	}
}

func TestSiteTemplateCanKeepRouteDisabled(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "templates", "site-compose.yml.tmpl")
	tpl, err := template.ParseFiles(path)
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := tpl.Execute(&rendered, siteTemplateData{SiteID: "site-id", Domain: "demo.juheai.club", RouteEnabled: false}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "traefik.enable=false") {
		t.Fatal("disabled site route was not rendered")
	}
}

func TestPrepareImagesSkipsImagesAlreadyPresentLocally(t *testing.T) {
	runner := &recordingRunner{handler: func(_ string, args []string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "config --format json") {
			return validComposeConfigJSON(), nil
		}
		return "", nil
	}}
	executor := &siteExecutor{runner: runner}
	dir := filepath.Join("tmp", "site-id")
	if err := executor.prepareImages(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.calls {
		if len(call.args) > 0 && call.args[0] == "pull" {
			t.Fatalf("locally available image was pulled: %v", call.args)
		}
	}
}

func TestPrepareImagesPullsOnlyMissingImages(t *testing.T) {
	runner := &recordingRunner{handler: func(_ string, args []string) (string, error) {
		command := strings.Join(args, " ")
		if strings.Contains(command, "config --format json") {
			return validComposeConfigJSON(), nil
		}
		if strings.HasPrefix(command, "image inspect example/site:v1") {
			return "", errors.New("not found")
		}
		return "", nil
	}}
	executor := &siteExecutor{runner: runner}
	if err := executor.prepareImages(context.Background(), filepath.Join("tmp", "site-id")); err != nil {
		t.Fatal(err)
	}
	pulled := false
	for _, call := range runner.calls {
		if strings.Join(call.args, " ") == "pull example/site:v1" {
			pulled = true
		}
	}
	if !pulled {
		t.Fatal("missing site image was not pulled")
	}
}

func TestComposeContractRejectsLoopbackWorkerOrigin(t *testing.T) {
	rendered := validComposeConfig()
	worker := rendered.Services["worker"]
	worker.Environment["VOZEB_PRO_WORKER_API_ORIGIN"] = "http://127.0.0.1:3000"
	rendered.Services["worker"] = worker
	if err := validateComposeContract(rendered); err == nil {
		t.Fatal("loopback worker origin was accepted")
	}
}

func TestComposeContractRejectsDatabaseCredentialsInWorker(t *testing.T) {
	rendered := validComposeConfig()
	worker := rendered.Services["worker"]
	worker.Environment["DATABASE_URL"] = "postgres://should-not-be-here"
	rendered.Services["worker"] = worker
	if err := validateComposeContract(rendered); err == nil {
		t.Fatal("worker database credential leak was accepted")
	}
}

func TestComposeContractRejectsControlDatabase(t *testing.T) {
	rendered := validComposeConfig()
	app := rendered.Services["app"]
	reporter := rendered.Services["reporter"]
	app.Environment["DATABASE_URL"] = "postgres://control:password@db/control_center"
	reporter.Environment["DATABASE_URL"] = app.Environment["DATABASE_URL"]
	rendered.Services["app"] = app
	rendered.Services["reporter"] = reporter
	if err := validateComposeContract(rendered); err == nil {
		t.Fatal("control database was accepted as a site database")
	}
}

func validComposeConfigJSON() string {
	return `{"services":{"app":{"image":"example/site:v1","container_name":"site-11111111-1111-4111-8111-111111111111-app","environment":{"VOZEB_PRO_DATABASE_PROVIDER":"postgres","DATABASE_URL":"postgres://user_11111111111141118111111111111111:password@db/site_11111111111141118111111111111111","VOZEB_PRO_INTERNAL_ORIGIN":"http://127.0.0.1:3000","VOZEB_PRO_MAINTENANCE_TOKEN":"12345678901234567890123456789012"}},"worker":{"image":"example/site:v1","environment":{"VOZEB_PRO_WORKER_API_ORIGIN":"http://app:3000","VOZEB_PRO_MAINTENANCE_TOKEN":"12345678901234567890123456789012"}},"reporter":{"image":"example/reporter:v1","environment":{"DATABASE_URL":"postgres://user_11111111111141118111111111111111:password@db/site_11111111111141118111111111111111","SITE_DATABASE_TABLE_PREFIX":"vozeb_pro_"}}}}`
}

func validComposeConfig() renderedComposeConfig {
	var rendered renderedComposeConfig
	if err := json.Unmarshal([]byte(validComposeConfigJSON()), &rendered); err != nil {
		panic(err)
	}
	return rendered
}

func TestRouteProbeUsesExactHostAndSiteIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "demo.juheai.club" {
			t.Errorf("unexpected host header: %q", r.Host)
			http.Error(w, "wrong host", http.StatusBadRequest)
			return
		}
		w.Header().Set("X-Control-Site-ID", "site-id")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	executor := &siteExecutor{cfg: config.Config{SiteRouterURL: server.URL}}
	if err := executor.probeRoute(context.Background(), siteRuntime{ID: "site-id", Domain: "demo.juheai.club"}, false); err != nil {
		t.Fatal(err)
	}
}

func TestRouteProbeRejectsAnotherSite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Control-Site-ID", "another-site")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	executor := &siteExecutor{cfg: config.Config{SiteRouterURL: server.URL}}
	if err := executor.probeRoute(context.Background(), siteRuntime{ID: "site-id", Domain: "demo.juheai.club"}, false); err == nil {
		t.Fatal("route probe accepted another site's identity")
	}
}

func TestSQLQuoting(t *testing.T) {
	if got := quoteIdentifier(`site"name`); got != `"site""name"` {
		t.Fatalf("unexpected identifier quoting: %s", got)
	}
	if got := quoteLiteral("it's"); got != "'it''s'" {
		t.Fatalf("unexpected literal quoting: %s", got)
	}
}

func TestDestroyRuntimeUsesOnlyDerivedSiteObjects(t *testing.T) {
	const siteID = "d149a773-0d3b-4d2c-863f-9fab4a79c1b0"
	runner := &recordingRunner{handler: func(_ string, args []string) (string, error) {
		return "Error: No such object", errors.New("not found")
	}}
	executor := &siteExecutor{runner: runner}
	if err := executor.destroyRuntime(context.Background(), siteRuntime{ID: siteID}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"container inspect site-" + siteID + "-app",
		"container inspect site-" + siteID + "-worker",
		"container inspect site-" + siteID + "-reporter",
		"volume inspect site-" + siteID + "-data",
	}
	if len(runner.calls) != len(want) {
		t.Fatalf("unexpected docker call count: %d", len(runner.calls))
	}
	for index, call := range runner.calls {
		if got := strings.Join(call.args, " "); got != want[index] {
			t.Fatalf("unexpected docker call %d: %s", index, got)
		}
	}
}

func TestRemoveSiteDirectoryRejectsUnsafeIdentifier(t *testing.T) {
	base := t.TempDir()
	marker := filepath.Join(base, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	executor := &siteExecutor{cfg: config.Config{SiteBaseDir: base}}
	if err := executor.removeSiteDirectory(".." + string(filepath.Separator)); err == nil {
		t.Fatal("unsafe site identifier was accepted")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("base directory was modified")
	}
}

func TestRemoveSiteDirectoryRemovesExactUUIDDirectory(t *testing.T) {
	const siteID = "d149a773-0d3b-4d2c-863f-9fab4a79c1b0"
	base := t.TempDir()
	target := filepath.Join(base, siteID)
	if err := os.MkdirAll(target, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "compose.yml"), []byte("services: {}"), 0600); err != nil {
		t.Fatal(err)
	}
	executor := &siteExecutor{cfg: config.Config{SiteBaseDir: base}}
	if err := executor.removeSiteDirectory(siteID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("site directory still exists: %v", err)
	}
}
