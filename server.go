package mdhost

import (
	"bytes"
	"io"
	"net/http"
	"html/template"
	"encoding/json"
	"log"
	"strings"
	"io/ioutil"
	"github.com/zenazn/goji/web"
	"github.com/zenazn/goji/web/middleware"
	"github.com/zenazn/goji/bind"
	"github.com/zenazn/goji/graceful"

	"github.com/Lavos/casket"
	"github.com/russross/blackfriday"
)

var (
)

type Server struct {
	ContentStorer casket.ContentStorer
	Filer casket.Filer
	MarkdownTemplate *template.Template

	mux *web.Mux
	socket string
}

type Context struct {
	Filename string
	Content template.HTML
}

type ErrorResponse struct {
	ErrorCode string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type ExistsResponse struct {
	Exists bool `json:"exists"`
}

type RevisionResponse struct {
	SHA string `json:"sha"`
}

func JSONCopy (data interface{}, w io.Writer) {
	var b bytes.Buffer
	j := json.NewEncoder(&b)
	j.Encode(data)
	b.WriteTo(w)
}

func NewServer(c casket.ContentStorer, f casket.Filer, socket string, template_bytes []byte) *Server {
	t, _ := template.New("base").Parse(string(template_bytes))

	return &Server{c, f, t, web.New(), socket}
}

func (s *Server) Run() {
	s.mux.Use(middleware.EnvInit)
	s.mux.Use(s.Options)
	s.mux.Get("/f/*", s.GetLatestRevision)
	s.mux.Get("/r/:sha", s.GetRevision)
	s.mux.Get("/x/exists/*", s.Exists)
	s.mux.Get("/x/meta/*", s.GetFileMeta)
	s.mux.Options("/c/*", s.Noop)
	s.mux.Post("/c/*", s.CreateFile)
	s.mux.Put("/c/*", s.NewRevision)

	listener := bind.Socket(s.socket)
	log.Println("Starting Goji on", listener.Addr())

	graceful.HandleSignals()
	bind.Ready()

	err := graceful.Serve(listener, s.mux)

	if err != nil {
		log.Fatal(err)
	}

	graceful.Wait()
}

func (s *Server) Error(status_code int, error_code, error_message string, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status_code)
	JSONCopy(ErrorResponse{error_code, error_message}, w)
}

func (s *Server) Options(c *web.C, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "accept, content-type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH")

		h.ServeHTTP(w, r)
	})
}

// Noop is a http.Handler that does nothing.
func (s *Server) Noop(c web.C, w http.ResponseWriter, r *http.Request) {

}

func (s *Server) GetLatestRevision(c web.C, w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/f/")
	file, err := s.Filer.Get(name)

	log.Printf("file: %#v", file)

	if err != nil {
		s.Error(404, "file_notfound", "File was not found.", w)
		return
	}

	latest_sha, err := file.Latest()

	if err != nil {
		s.Error(500, "file_norevisions", "No revisions found for this file.", w)
		return
	}

	latest_revision, err := s.ContentStorer.Get(latest_sha)

	if err != nil {
		s.Error(500, "rev_notfound", "Revision not found.", w)
		return
	}

	s.MarkdownTemplate.Execute(w, Context{name, template.HTML(blackfriday.MarkdownCommon(latest_revision))})
}

func (s *Server) GetRevision(c web.C, w http.ResponseWriter, r *http.Request) {
	sha_string := c.URLParams["sha"]

	if sha_string == "" {
		s.Error(500, "sha_missing", "SHA missing.", w)
		return
	}

	sha := casket.NewSHA1SumFromString(sha_string)

	revision, err := s.ContentStorer.Get(sha)

	if err != nil {
		s.Error(500, "rev_notfound", "Revision not found.", w)
		return
	}

	w.Write(revision)
}

func (s *Server) Exists(c web.C, w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/x/exists/")
	exists, err := s.Filer.Exists(name)

	if err != nil {
		s.Error(500, "exists_err", "Could not detect if file exists.", w)
		return
	}

	JSONCopy(&ExistsResponse{exists}, w)
}

func (s *Server) GetFileMeta(c web.C, w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/x/meta/")
	file, err := s.Filer.Get(name)

	if err != nil {
		s.Error(404, "file_notfound", "File was not found.", w)
		return
	}

	JSONCopy(file, w)
}

func (s *Server) CreateFile(c web.C, w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/c/")
	file, err := s.Filer.NewFile(name, "text/markdown")

	if err != nil {
		s.Error(404, "file_creation", "Could not create new file.", w)
		return
	}

	JSONCopy(file, w)
}
func (s *Server) NewRevision(c web.C, w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/c/")
	file, err := s.Filer.Get(name)

	if err != nil {
		s.Error(404, "file_notfound", "File was not found.", w)
		return
	}

	b, err := ioutil.ReadAll(r.Body)
	r.Body.Close()

	if err != nil {
		s.Error(500, "io", "Could not read request body.", w)
		return
	}

	sha, err := s.ContentStorer.Put(b)

	if err != nil {
		s.Error(500, "storage", "Could not store revision bytes.", w)
		return
	}

	s.Filer.AddRevision(file, sha)
	JSONCopy(&RevisionResponse{ sha.String() }, w)
}
