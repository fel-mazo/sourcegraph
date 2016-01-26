// Package tmpl defines, loads, and renders the app's templates.
package tmpl

import (
	"fmt"
	htmpl "html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"github.com/justinas/nosurf"
	"github.com/sourcegraph/mux"
	"sourcegraph.com/sourcegraph/appdash"
	"src.sourcegraph.com/sourcegraph/app/appconf"
	appauth "src.sourcegraph.com/sourcegraph/app/auth"
	"src.sourcegraph.com/sourcegraph/app/internal/canonicalurl"
	"src.sourcegraph.com/sourcegraph/app/internal/returnto"
	tmpldata "src.sourcegraph.com/sourcegraph/app/templates"
	"src.sourcegraph.com/sourcegraph/conf"
	"src.sourcegraph.com/sourcegraph/conf/feature"
	"src.sourcegraph.com/sourcegraph/go-sourcegraph/sourcegraph"
	"src.sourcegraph.com/sourcegraph/util/eventsutil"
	"src.sourcegraph.com/sourcegraph/util/handlerutil"
	"src.sourcegraph.com/sourcegraph/util/httputil"
	"src.sourcegraph.com/sourcegraph/util/httputil/httpctx"
	"src.sourcegraph.com/sourcegraph/util/metricutil"
	"src.sourcegraph.com/sourcegraph/util/randstring"
	"src.sourcegraph.com/sourcegraph/util/traceutil"
)

var (
	templates   = map[string]*htmpl.Template{}
	templatesMu sync.Mutex
)

// Get gets a template by name, if it exists (and has previously been
// parsed, either by Load or by Add).
// Templates generally bare the name of the first file in their set.
func Get(name string) *htmpl.Template {
	templatesMu.Lock()
	t := templates[name]
	templatesMu.Unlock()
	return t
}

// Add adds a parsed template. It will be available to callers of Exec
// and Get.
//
// TODO(sqs): is this necessary?
func Add(name string, tmpl *htmpl.Template) {
	templatesMu.Lock()
	templates[name] = tmpl
	templatesMu.Unlock()
}

// Delete removes the named template.
func Delete(name string) {
	templatesMu.Lock()
	delete(templates, name)
	templatesMu.Unlock()
}

// repoTemplates returns all repository template pages if successful.
func repoTemplates() error {
	return parseHTMLTemplates([][]string{
		{"repo/main.html", "repo/readme.inc.html", "repo/tree.inc.html", "repo/tree/dir.inc.html", "repo/commit.inc.html"},
		{"repo/badges.html", "repo/badges_and_counters.html"},
		{"repo/counters.html", "repo/badges_and_counters.html"},
		{"repo/builds.html", "builds/build.inc.html"},
		{"repo/build.html", "builds/build.inc.html", "repo/commit.inc.html"},
		{"repo/tree/file.html"},
		{"repo/tree/doc.html", "repo/commit.inc.html"},
		{"repo/tree/dir.html", "repo/tree/dir.inc.html", "repo/commit.inc.html"},
		{"repo/search.html"},
		{"repo/frame.html", "error/common.html"},
		{"repo/commit.html", "repo/commit.inc.html"},
		{"repo/commits.html", "repo/commit.inc.html"},
		{"repo/branches.html"},
		{"repo/tags.html"},
		{"repo/compare.html", "repo/commit.inc.html"},
		{"repo/no_vcs_data.html"},

		{"def/examples.html", "def/examples.inc.html", "def/snippet.inc.html", "def/def.html"},
	}, []string{
		"repo/repo.html",
		"repo/subnav.html",

		"common.html",
		"layout.html",
		"nav.html",
		"footer.html",
	})
}

// commonTemplates returns all common templates such as user pages, search,
// etc. if successful.
func commonTemplates() error {
	return parseHTMLTemplates([][]string{
		{"user/login.html"},
		{"user/signup.html"},
		{"user/logged_out.html"},
		{"user/forgot_password.html"},
		{"user/password_reset.html"},
		{"user/new_password.html"},
		{"user/settings/profile.html", "user/settings/common.inc.html"},
		{"user/settings/notifications.html", "user/settings/common.inc.html"},
		{"user/settings/keys.html", "user/settings/common.inc.html"},

		{"home/dashboard.html"},

		{"builds/builds.html", "builds/build.inc.html"},
		{"coverage/coverage.html"},
		{"error/error.html", "error/common.html"},

		{"oauth-provider/authorize.html"},

		{"app_global.html"},
	}, []string{
		"common.html",
		"layout.html",
		"nav.html",
		"footer.html",
	})
}

// standaloneTemplates returns a set of standalone templates if
// successful.
func standaloneTemplates() error {
	return parseHTMLTemplates([][]string{
		{"def/popover.html"},
	}, []string{"common.html"})
}

// Load loads (or re-loads) all template files from disk.
func Load() {
	if err := repoTemplates(); err != nil {
		log.Fatal(err)
	}
	if err := commonTemplates(); err != nil {
		log.Fatal(err)
	}
	if err := standaloneTemplates(); err != nil {
		log.Fatal(err)
	}
}

// Common holds fields that are available at the top level in every
// template executed by Exec.
type Common struct {
	RequestHost string // the request's Host header

	Session   *appauth.Session // the session cookie
	CSRFToken string

	CurrentUser   *sourcegraph.User
	UserEmails    *sourcegraph.EmailAddrList
	CurrentRoute  string
	CurrentURI    *url.URL
	CurrentURL    *url.URL
	CurrentQuery  url.Values
	CurrentSpanID appdash.SpanID

	// TemplateName is the filename of the template being rendered
	// (e.g., "repo/main.html").
	TemplateName string

	// AppURL is the conf.AppURL(ctx) value for the current context.
	AppURL       *url.URL
	CanonicalURL *url.URL
	HostName     string

	Ctx context.Context

	CurrentRouteVars map[string]string

	// Debug is whether to show debugging info on the rendered page.
	Debug bool

	// ReturnTo is the URL to the page that the user should be returned to if
	// the user initiates a signup or login process from this page. Usually this
	// is the same as CurrentURI. The exceptions are when there are tracking
	// querystring parameters (we want to remove from the URL that the user
	// visits after signing up), and when the user is on the signup or login
	// pages themselves (otherwise we could get into a loop).
	//
	// The ReturnTo field is overridden by serveSignUp and other handlers that want
	// to set a ReturnTo different from CurrentURI.
	ReturnTo string

	// ExternalLinks decides if we should include links to things like
	// sourcegraph.com and the issue tracker on github.com
	DisableExternalLinks bool

	// Features is a struct containing feature toggles. See conf/feature
	Features interface{}

	// ErrorID is a randomly generated string used to identify a specific instance
	// of app error in the error logs.
	ErrorID string

	// CacheControl is the HTTP cache-control header value that should be set in all
	// AJAX requests originating from this page.
	CacheControl string

	// HideMOTD, if true, prevents the MOTD (message of the day) from
	// being displayed at the top of the template.
	HideMOTD bool

	// HideSearch, if set, hides the search bar from the top
	// navigation bar.
	HideSearch bool
}

func executeTemplateBase(w http.ResponseWriter, templateName string, data interface{}) error {
	t := Get(templateName)
	if t == nil {
		return fmt.Errorf("Template %s not found", templateName)
	}
	return t.Execute(w, data)
}

// Exec executes the template (named by `name`) using the template data.
func Exec(req *http.Request, resp http.ResponseWriter, name string, status int, header http.Header, data interface{}) error {
	ctx := httpctx.FromRequest(req)
	currentUser := handlerutil.UserFromRequest(req)

	appEvent := &sourcegraph.UserEvent{
		Type:    "app",
		Service: conf.AppURL(ctx).String(),
		Method:  name,
		Result:  strconv.Itoa(status),
		URL:     req.URL.String(),
	}
	if currentUser != nil {
		appEvent.UID = currentUser.UID
	}

	if data != nil {
		sess, err := appauth.ReadSessionCookie(req)
		if err != nil && err != appauth.ErrNoSession {
			return err
		}

		field := reflect.ValueOf(data).Elem().FieldByName("Common")
		existingCommon := field.Interface().(Common)

		currentURL := conf.AppURL(ctx).ResolveReference(req.URL)
		canonicalURL := existingCommon.CanonicalURL
		if canonicalURL == nil {
			canonicalURL = canonicalurl.FromURL(currentURL)
		}

		returnTo, _ := returnto.BestGuess(req)

		var errorID string
		errField := reflect.ValueOf(data).Elem().FieldByName("Err")
		if errField.IsValid() {
			errorID = randstring.NewLen(6)
			appError := errField.Interface().(error)
			appEvent.Message = fmt.Sprintf("ErrorID:%s Msg:%s", errorID, appError.Error())
		}

		// Propagate Cache-Control no-cache and max-age=0 directives
		// to the requests made by our client-side JavaScript. This is
		// not a perfect parser, but it catches the important cases.
		var cacheControl string
		if cc := req.Header.Get("cache-control"); strings.Contains(cc, "no-cache") || strings.Contains(cc, "max-age=0") {
			cacheControl = "no-cache"
		}

		field.Set(reflect.ValueOf(Common{
			CurrentUser: handlerutil.FullUserFromRequest(req),
			UserEmails:  handlerutil.EmailsFromRequest(req),

			RequestHost: req.Host,

			Session:   sess,
			CSRFToken: nosurf.Token(req),

			TemplateName: name,

			CurrentRoute: httpctx.RouteName(req),
			CurrentURI:   req.URL,
			CurrentURL:   currentURL,
			CurrentQuery: req.URL.Query(),

			AppURL:       conf.AppURL(ctx),
			CanonicalURL: canonicalURL,

			Ctx: ctx,

			CurrentSpanID:    traceutil.SpanID(req),
			CurrentRouteVars: mux.Vars(req),
			Debug:            handlerutil.DebugMode(req),
			ReturnTo:         returnTo,

			DisableExternalLinks: appconf.Flags.DisableExternalLinks,
			Features:             feature.Features,

			ErrorID: errorID,

			CacheControl: cacheControl,

			HideMOTD: existingCommon.HideMOTD,
		}))
	}

	metricutil.LogEvent(ctx, appEvent)
	eventsutil.LogPageView(ctx, currentUser, req)

	// Buffer HTTP response so that if the template execution returns
	// an error (e.g., a template calls a template func that panics or
	// returns an error), we can return an HTTP error status code and
	// page to the browser. If we don't buffer it here, then the HTTP
	// response is already partially written to the client by the time
	// the error is detected, so the page rendering is aborted halfway
	// through with an error message, AND the HTTP status is 200
	// (which makes it hard to detect failures in tests).
	var bw httputil.ResponseBuffer

	for k, v := range header {
		bw.Header()[k] = v
	}
	if ct := bw.Header().Get("content-type"); ct == "" {
		bw.Header().Set("Content-Type", "text/html; charset=utf-8")
	}

	bw.WriteHeader(status)
	if status == http.StatusNotModified {
		return nil
	}

	if err := executeTemplateBase(&bw, name, data); err != nil {
		return err
	}

	return bw.WriteTo(resp)
}

// parseHTMLTemplates takes a list of template file sets. For each set in the
// list it creates a template containing all of the definitions found in that
// set. The name of each template will be the same as the first file in each
// set.
//
// A list of layout templates may also be provided. These will be shared
// amongst all templates.
func parseHTMLTemplates(sets [][]string, layout []string) error {
	var wg sync.WaitGroup
	for _, setv := range sets {
		set := setv
		if layout != nil {
			set = append(setv, layout...)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()

			t := htmpl.New("")
			t.Funcs(FuncMap)

			for _, tname := range set {
				f, err := tmpldata.Data.Open("/" + tname)
				if err != nil {
					log.Fatalf("read template asset %s: %s", tname, err)
				}
				tmpl, err := ioutil.ReadAll(f)
				f.Close()
				if err != nil {
					log.Fatalf("read template asset %s: %s", tname, err)
				}
				if _, err := t.Parse(string(tmpl)); err != nil {
					log.Fatalf("template %v: %s", set, err)
				}
			}

			t = t.Lookup("ROOT")
			if t == nil {
				log.Fatalf("ROOT template not found in %v", set)
			}
			Add(set[0], t)
		}()
	}
	wg.Wait()
	return nil
}

// FuncMap is the template func map passed to each template.
var FuncMap htmpl.FuncMap
