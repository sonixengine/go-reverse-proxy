package main

import (
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/jpillora/ipfilter"

	log "go-reverse-proxy/log"
)

// Server struct file
type Server struct {
	Name     string   `json:"name"`
	Listen   string   `json:"listen"`
	Domains  []string `json:"domains"`
	Root     *string  `json:"root"`
	SSL      bool     `json:"ssl"`
	GZIP     bool     `json:"gzip"`
	GFW      bool     `json:"gfw"`
	Files    []Files  `json:"files"`
	Proxies  []Proxy  `json:"proxies"`
	KeyFile  string   `json:"key_file"`
	CertFile string   `json:"cert_file"`
}

type Files struct {
	Path     string
	Location string
	Index    string
}

// Keys Return keys of the given map
func Keys(m map[string]string) map[string]interface{} {
	po := make(map[string]interface{})
	for k := range m {
		po[k] = m[k]
	}
	return po
}

// spaHandler implements the http.Handler interface, so we can use it
// to respond to HTTP requests. The path to the static directory and
// path to the index file within that static directory are used to
// serve the SPA in the given static directory.
type spaHandler struct {
	staticPath string
	indexPath  string
}

// ServeHTTP inspects the URL path to locate a file within the static dir
// on the SPA handler. If a file is found, it will be served. If not, the
// file located at the index path on the SPA handler will be served. This
// is suitable behavior for serving an SPA (single page application).
func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the absolute path to prevent directory traversal
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	path := filepath.Dir(ex)
	if err != nil {
		// if we failed to get the absolute path respond with a 400 bad request
		// and stop
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// prepend the path with the path to the static directory
	path = filepath.Join(path, h.staticPath)
	checkPath := filepath.Join(path, r.URL.Path)

	// check whether a file exists at the given path
	_, err = os.Stat(checkPath)
	if os.IsNotExist(err) {
		// file does not exist, serve index.html
		http.ServeFile(w, r, filepath.Join(path, h.indexPath))
		return
	} else if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) stating the
		// file, return a 500 internal server error and stop
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// otherwise, use http.FileServer to serve the static dir
	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

// Start the server
func (s *Server) Start(conf *Config, f *ipfilter.IPFilter) {

	if s.Root != nil {
		log.Info("%s listen %s, ssl: %v, static dir %s", s.Name, s.Listen, s.SSL, *s.Root)
	}

	r := mux.NewRouter()

	if s.Files != nil {
		index := "index.html"
		for _, files := range s.Files {
			pathLocation := files.Location
			if files.Index != "" {
				index = files.Index
			}
			spa := spaHandler{staticPath: pathLocation, indexPath: index}
			r.PathPrefix(files.Path).Subrouter()
			r.PathPrefix(files.Path).Subrouter().HandleFunc("", spa.ServeHTTP)
			r.PathPrefix(files.Path).Subrouter().HandleFunc("/", spa.ServeHTTP)
			log.Info("%s listen %s, ssl: %v, proxy from %s ==> %s", s.Name, s.Listen, s.SSL, index, files.Path)
		}
	}

	for _, proxy := range s.Proxies {

		if proxy.ProxyPass != nil {
			log.Info("%s listen %s, ssl: %v, proxy to %s ==> %s", s.Name, s.Listen, s.SSL, *proxy.ProxyPass, *proxy.ProxyPath)
		}

		r.PathPrefix(*proxy.ProxyPath).Subrouter()
		r.PathPrefix(*proxy.ProxyPath).Subrouter().HandleFunc("", proxy.setup)
		r.PathPrefix(*proxy.ProxyPath).Subrouter().HandleFunc("/", proxy.setup)
		r.PathPrefix(*proxy.ProxyPath).Subrouter().HandleFunc(`/{rest:[a-zA-Z0-9=.?\-~_\/]+}`, proxy.setup)
	}

	if s.Root != nil {
		pathLocation := *s.Root
		log.Info("Config location %s", pathLocation)
		spa := spaHandler{staticPath: pathLocation, indexPath: "index.html"}
		r.PathPrefix("/").Handler(spa)
	}

	port := getenv("PORT", s.Listen)

	myProtectedHandler := f.Wrap(r)

	var err error
	if s.SSL {
		err = http.ListenAndServeTLS("0.0.0.0:"+port, s.CertFile, s.KeyFile, myProtectedHandler)
	} else {
		err = http.ListenAndServe("0.0.0.0:"+port, myProtectedHandler)
	}

	if err != nil {
		log.Error("%v", err)
	}
}

var transport = &http.Transport{
	ResponseHeaderTimeout: 30 * time.Second,
	TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
}
